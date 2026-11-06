package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	HostedEmailLogin = "login"
	HostedEmailReset = "reset"
)

var ErrHostedEmailTokenInvalid = errors.New("invalid or expired email link")

// HostedEmailDelivery is returned only over the authenticated internal API.
type HostedEmailDelivery struct {
	Token string `json:"token,omitempty"`
	Email string `json:"email,omitempty"`
}

func validHostedEmailPurpose(purpose string) bool {
	return purpose == HostedEmailLogin || purpose == HostedEmailReset
}

func (s *HostedLoginService) IssueEmailToken(ctx context.Context, email, purpose, destination string) (*HostedEmailDelivery, error) {
	if !validHostedEmailPurpose(purpose) || (destination != "portal" && destination != "account") {
		return nil, ErrHostedEmailTokenInvalid
	}
	if s == nil || s.handoffSecret == "" {
		return nil, errors.New("hosted email authentication unavailable")
	}
	match, err := s.lookupUser(email)
	if err != nil {
		if isEmailIdentityError(err) {
			return &HostedEmailDelivery{}, nil
		}
		return nil, err
	}
	if _, err := s.redirectURL(match); err != nil {
		if isEmailIdentityError(err) {
			return &HostedEmailDelivery{}, nil
		}
		return nil, err
	}
	code, err := generateHostedLoginHandoffCode()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	record := models.HostedEmailToken{
		TenantID: match.TenantID, UserID: match.UserID, Purpose: purpose,
		TokenHash:      hashHostedLoginHandoff(s.handoffSecret, "email:"+purpose+":"+code),
		CredentialHash: hashHostedLoginHandoff(s.handoffSecret, "credential:"+strings.ToLower(strings.TrimSpace(email))+":"+match.PasswordHash),
		Destination:    destination, ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now,
	}
	// Atomic cooldown, including requests made concurrently or by different IPs.
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "purpose"}},
		DoUpdates: clause.AssignmentColumns([]string{"token_hash", "credential_hash", "destination", "expires_at", "created_at", "used_at"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Lt{Column: "hosted_email_tokens.created_at", Value: now.Add(-time.Minute)}}},
	}).Create(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return &HostedEmailDelivery{}, nil
	}
	return &HostedEmailDelivery{Token: code, Email: strings.ToLower(strings.TrimSpace(email))}, nil
}

func isEmailIdentityError(err error) bool {
	return errors.Is(err, ErrHostedLoginInvalidCredentials) || errors.Is(err, ErrHostedLoginAmbiguousEmail) || errors.Is(err, ErrHostedLoginTenantInactive) || errors.Is(err, ErrHostedLoginRedirectUnavailable)
}

// CompleteEmailToken consumes and acts in a single transaction. GET requests
// never call this method, so email security scanners cannot consume a link.
func (s *HostedLoginService) CompleteEmailToken(ctx context.Context, token, purpose, password string) (*HostedLoginResult, string, error) {
	if s == nil || s.handoffSecret == "" || !validHostedEmailPurpose(purpose) || len(token) != 43 {
		return nil, "", ErrHostedEmailTokenInvalid
	}
	var passwordHash string
	if purpose == HostedEmailReset {
		if len(password) < 12 || len(password) > 72 {
			return nil, "", errors.New("password must contain 12 to 72 bytes")
		}
		var err error
		passwordHash, err = auth.HashPassword(password)
		if err != nil {
			return nil, "", err
		}
	}
	var result *HostedLoginResult
	var destination string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var record models.HostedEmailToken
		if err := tx.Where("token_hash = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?", hashHostedLoginHandoff(s.handoffSecret, "email:"+purpose+":"+token), purpose, now).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrHostedEmailTokenInvalid
			}
			return err
		}
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", record.TenantID, record.UserID).First(&user).Error; err != nil {
			return ErrHostedEmailTokenInvalid
		}
		if record.CredentialHash != hashHostedLoginHandoff(s.handoffSecret, "credential:"+strings.ToLower(strings.TrimSpace(user.Email))+":"+user.PasswordHash) {
			return ErrHostedEmailTokenInvalid
		}
		service := NewHostedLoginService(tx, s.fallbackBaseURL, s.handoffSecret)
		identity, err := service.Resolve(user.Email)
		if err != nil || identity.UserID != record.UserID || identity.TenantID != record.TenantID {
			return ErrHostedEmailTokenInvalid
		}
		claimed := tx.Model(&models.HostedEmailToken{}).Where("id = ? AND token_hash = ? AND used_at IS NULL AND expires_at > ?", record.ID, record.TokenHash, now).Update("used_at", now)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return ErrHostedEmailTokenInvalid
		}
		destination = record.Destination
		if purpose == HostedEmailLogin {
			result, err = service.IssueHandoff(*identity)
			return err
		}
		if err := tx.Model(&models.User{}).Where("tenant_id = ? AND id = ?", user.TenantID, user.ID).Updates(map[string]any{"password_hash": passwordHash, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.HostedEmailToken{}).Where("tenant_id = ? AND user_id = ? AND used_at IS NULL", user.TenantID, user.ID).Update("used_at", now).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.PasswordResetToken{}).Where("tenant_id = ? AND user_id = ? AND used_at IS NULL", user.TenantID, user.ID).Update("used_at", now).Error; err != nil {
			return err
		}
		return (&auth.SessionStore{DB: tx}).RevokeUser(ctx, user.TenantID, user.ID)
	})
	return result, destination, err
}
