package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
)

var ErrPasskeySignupInvalid = errors.New("invalid or expired passkey signup")

type signupPasskeyUser struct{ id, email, name string }

func (u signupPasskeyUser) WebAuthnID() []byte                         { return []byte(u.id) }
func (u signupPasskeyUser) WebAuthnName() string                       { return u.email }
func (u signupPasskeyUser) WebAuthnDisplayName() string                { return u.name }
func (u signupPasskeyUser) WebAuthnCredentials() []webauthn.Credential { return nil }

type PasskeySignupService struct {
	db     *gorm.DB
	origin string
}

func NewPasskeySignupService(db *gorm.DB, origin string) *PasskeySignupService {
	return &PasskeySignupService{db: db, origin: strings.TrimRight(origin, "/")}
}

func (s *PasskeySignupService) webAuthn() (*webauthn.WebAuthn, error) {
	u, err := url.Parse(s.origin)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("hosted passkey origin is not configured")
	}
	isLocal := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocal) {
		return nil, errors.New("hosted passkey origin is not configured")
	}
	return webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPDisplayName: "ClientShare", RPOrigins: []string{s.origin},
		Timeouts: webauthn.TimeoutsConfig{Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute}}})
}

func signupTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *PasskeySignupService) Begin(ctx context.Context, email, name, slug string) (map[string]any, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if address, err := mail.ParseAddress(email); err != nil || address.Address != email || strings.TrimSpace(slug) == "" {
		return nil, ErrPasskeySignupInvalid
	}
	w, err := s.webAuthn()
	if err != nil {
		return nil, err
	}
	user := signupPasskeyUser{id: ids.NewULID(), email: email, name: strings.TrimSpace(name)}
	if user.name == "" {
		user.name = email
	}
	creation, session, err := w.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	record := models.PasskeySignup{ID: ids.NewULID(), TokenHash: signupTokenHash(token), Email: email,
		TenantSlug: strings.ToLower(strings.TrimSpace(slug)), UserID: user.id, SessionData: data,
		RPID: session.RelyingPartyID, ExpiresAt: time.Now().Add(5 * time.Minute)}
	if err := s.db.WithContext(ctx).Where("reserved = ? AND expires_at < ?", false, time.Now()).Delete(&models.PasskeySignup{}).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return map[string]any{"signup_token": token, "public_key": creation.Response}, nil
}

func (s *PasskeySignupService) Verify(ctx context.Context, token string, response json.RawMessage) error {
	var record models.PasskeySignup
	if err := s.db.WithContext(ctx).Where("token_hash = ? AND verified_at IS NULL AND expires_at > ?", signupTokenHash(token), time.Now()).First(&record).Error; err != nil {
		return ErrPasskeySignupInvalid
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(record.SessionData, &session); err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return ErrPasskeySignupInvalid
	}
	w, err := s.webAuthn()
	if err != nil {
		return err
	}
	credential, err := w.CreateCredential(signupPasskeyUser{id: record.UserID, email: record.Email, name: record.Email}, session, parsed)
	if err != nil {
		return ErrPasskeySignupInvalid
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&models.PasskeySignup{}).Where("id = ? AND verified_at IS NULL AND expires_at > ?", record.ID, time.Now()).Updates(map[string]any{
		"credential_data": data, "verified_at": time.Now(), "expires_at": time.Now().Add(24 * time.Hour),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPasskeySignupInvalid
	}
	return nil
}

// Reserve is internal-only. Bind the proof to the email and workspace before SaaS
// persists it for asynchronous provisioning. It does not establish a login session.
func (s *PasskeySignupService) Reserve(ctx context.Context, token, email, slug string) error {
	result := s.db.WithContext(ctx).Model(&models.PasskeySignup{}).Where("token_hash = ? AND email = ? AND tenant_slug = ? AND verified_at IS NOT NULL AND (reserved = ? OR expires_at > ?)",
		signupTokenHash(token), strings.ToLower(strings.TrimSpace(email)), strings.ToLower(strings.TrimSpace(slug)), true, time.Now()).Update("reserved", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPasskeySignupInvalid
	}
	return nil
}

// createPasskeyAdmin runs inside the provisioning transaction. The preallocated
// user ID preserves WebAuthn's user handle. A token cannot be used in another tenant.
func createPasskeyAdmin(tx *gorm.DB, tenantID string, input CreateAdminUserInput) (*models.User, error) {
	var tenant models.Tenant
	if err := tx.First(&tenant, "id = ?", tenantID).Error; err != nil {
		return nil, err
	}
	var record models.PasskeySignup
	if err := tx.Where("token_hash = ? AND email = ? AND tenant_slug = ? AND reserved = ? AND verified_at IS NOT NULL",
		signupTokenHash(input.PasskeySignupToken), strings.ToLower(strings.TrimSpace(input.Email)), tenant.Slug, true).First(&record).Error; err != nil {
		return nil, ErrPasskeySignupInvalid
	}
	if record.TenantID == tenantID {
		var user models.User
		err := tx.First(&user, "id = ? AND tenant_id = ?", record.UserID, tenantID).Error
		return &user, err
	}
	if record.TenantID != "" {
		return nil, ErrPasskeySignupInvalid
	}
	result := tx.Model(&record).Where("tenant_id = ''").Update("tenant_id", tenantID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrPasskeySignupInvalid
	}
	user := models.User{ID: record.UserID, TenantID: tenantID, Email: record.Email, Name: strings.TrimSpace(input.Name), Role: "admin"}
	if err := tx.Create(&user).Error; err != nil {
		return nil, err
	}
	var credential webauthn.Credential
	if err := json.Unmarshal(record.CredentialData, &credential); err != nil {
		return nil, err
	}
	stored := models.PasskeyCredential{TenantID: tenantID, UserID: user.ID, CredentialID: base64.RawURLEncoding.EncodeToString(credential.ID),
		CredentialIDHash: signupTokenHash(string(credential.ID)), CredentialData: record.CredentialData, RPID: record.RPID, Name: "Signup passkey", CreatedAt: time.Now()}
	if err := tx.Create(&stored).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
