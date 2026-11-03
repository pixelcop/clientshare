package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"time"
)

const SessionCookie = "clientshare_session"
const PortalAudience = "portal"
const AccountAudience = "account"
const AuthExpiry = 30 * 24 * time.Hour
const IdleExpiry = 24 * time.Hour

var ErrSessionInvalid = errors.New("invalid or expired session")

type SessionStore struct{ DB *gorm.DB }
type SessionIdentity struct {
	SessionID  string `json:"session_id"`
	GroupID    string `json:"-"`
	TenantID   string `json:"tenant_id"`
	UserID     string `json:"user_id"`
	Role       string `json:"role"`
	LinkID     string `json:"link_id,omitempty"`
	ClientID   string `json:"client_id,omitempty"`
	AccessType string `json:"access_type,omitempty"`
}

func SessionHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *SessionStore) Create(ctx context.Context, tenantID, userID, linkID, audience, groupID, previous string) (string, error) {
	if tenantID == "" || (userID == "") == (linkID == "") || (audience != PortalAudience && audience != AccountAudience) || (audience == AccountAudience && linkID != "") {
		return "", ErrSessionInvalid
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	now := time.Now().UTC()
	record := models.BrowserSession{TokenHash: SessionHash(raw), TenantID: tenantID, UserID: userID, LinkID: linkID, Audience: audience, GroupID: groupID, ExpiresAt: now.Add(AuthExpiry), IdleExpiresAt: now.Add(IdleExpiry)}
	if audience == AccountAudience {
		record.ExpiresAt = now.Add(IdleExpiry)
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if previous != "" {
			var old models.BrowserSession
			err := tx.Where("token_hash = ? AND audience = ?", SessionHash(previous), audience).First(&old).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && (old.TenantID != tenantID || old.UserID != userID || old.LinkID != linkID) {
				// An identity switch must not retain billing access belonging to
				// the previous portal identity, or an outstanding handoff to it.
				if err := (&SessionStore{DB: tx}).Revoke(ctx, previous, audience, true); err != nil {
					return err
				}
			} else if err == nil && record.GroupID == "" && old.RevokedAt == nil && old.ExpiresAt.After(now) && old.IdleExpiresAt.After(now) {
				record.GroupID = old.GroupID
			}
			if err := tx.Model(&models.BrowserSession{}).Where("token_hash = ? AND audience = ?", SessionHash(previous), audience).Update("revoked_at", now).Error; err != nil {
				return err
			}
		}
		return tx.Create(&record).Error
	})
	return raw, err
}

func (s *SessionStore) Validate(ctx context.Context, raw, audience, tenantID string) (*SessionIdentity, error) {
	if len(raw) != 43 {
		return nil, ErrSessionInvalid
	}
	now := time.Now().UTC()
	var record models.BrowserSession
	query := s.DB.WithContext(ctx).Where("token_hash = ? AND audience = ? AND revoked_at IS NULL AND expires_at > ? AND idle_expires_at > ?", SessionHash(raw), audience, now, now)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.First(&record).Error; err != nil {
		return nil, sessionError(err)
	}
	identity := &SessionIdentity{SessionID: record.ID, GroupID: record.GroupID, TenantID: record.TenantID, UserID: record.UserID, LinkID: record.LinkID}
	var entitlement models.TenantEntitlement
	err := s.DB.WithContext(ctx).Where("tenant_id = ?", record.TenantID).First(&entitlement).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Billing remains available to owners of suspended workspaces so they can
	// manage their subscription. Portal access does not.
	if audience == PortalAudience && err == nil && entitlement.Status != "active" {
		return nil, ErrSessionInvalid
	}
	if record.UserID != "" {
		var user models.User
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", record.TenantID, record.UserID).First(&user).Error; err != nil {
			return nil, sessionError(err)
		}
		identity.Role = user.Role
	} else {
		var link models.SecureLink
		if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", record.TenantID, record.LinkID).First(&link).Error; err != nil {
			return nil, sessionError(err)
		}
		if !link.ExpiresAt.After(now) {
			return nil, ErrSessionInvalid
		}
		identity.Role, identity.ClientID, identity.AccessType = "link", link.ClientID, link.AccessType
	}
	// Throttle writes without caching validity or delaying revocation.
	if record.IdleExpiresAt.Before(now.Add(IdleExpiry - time.Hour)) {
		if err := s.DB.WithContext(ctx).Model(&models.BrowserSession{}).Where("id = ? AND revoked_at IS NULL", record.ID).Update("idle_expires_at", now.Add(IdleExpiry)).Error; err != nil {
			return nil, err
		}
	}
	return identity, nil
}

func sessionError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSessionInvalid
	}
	return err
}

func (s *SessionStore) Revoke(ctx context.Context, raw, audience string, group bool) error {
	query := s.DB.WithContext(ctx).Model(&models.BrowserSession{}).Where("token_hash = ? AND audience = ?", SessionHash(raw), audience)
	if group {
		var record models.BrowserSession
		if err := s.DB.WithContext(ctx).Where("token_hash = ? AND audience = ?", SessionHash(raw), audience).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("session_group_id = ?", record.GroupID).Delete(&models.HostedLoginHandoff{}).Error; err != nil {
				return err
			}
			return tx.Model(&models.BrowserSession{}).Where("group_id = ?", record.GroupID).Update("revoked_at", time.Now().UTC()).Error
		})
	}
	return query.Update("revoked_at", time.Now().UTC()).Error
}

func (s *SessionStore) RevokeUser(ctx context.Context, tenantID, userID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&models.HostedLoginHandoff{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&models.WebAuthnChallenge{}).Error; err != nil {
			return err
		}
		return tx.Model(&models.BrowserSession{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Update("revoked_at", time.Now().UTC()).Error
	})
}

func Store(c fiber.Ctx) *SessionStore { s, _ := c.Locals("session_store").(*SessionStore); return s }
func SetSessionCookie(c fiber.Ctx, raw string) {
	c.Cookie(&fiber.Cookie{Name: SessionCookie, Value: raw, Path: "/", HTTPOnly: true, Secure: true, SameSite: "Lax", Expires: time.Now().Add(AuthExpiry)})
	// Explicit cutover: old JWTs are never exchanged for new sessions.
	c.Cookie(&fiber.Cookie{Name: "jwt", Path: "/", HTTPOnly: true, Secure: true, SameSite: "Lax", MaxAge: -1, Expires: time.Unix(0, 0)})
}
