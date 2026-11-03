package auth

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func sessionTestStore(t *testing.T) *SessionStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sessions.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BrowserSession{}, &models.User{}, &models.TenantEntitlement{}, &models.SecureLink{}, &models.HostedLoginHandoff{}, &models.WebAuthnChallenge{}))
	require.NoError(t, db.Create(&models.User{ID: "user", TenantID: "tenant", Email: "user@example.test", Role: "admin"}).Error)
	return &SessionStore{DB: db}
}

func TestSessionIsolationRotationAndRevocation(t *testing.T) {
	s := sessionTestStore(t)
	ctx := t.Context()
	account, err := s.Create(ctx, "tenant", "user", "", AccountAudience, "", "")
	require.NoError(t, err)
	identity, err := s.Validate(ctx, account, AccountAudience, "")
	require.NoError(t, err)
	portal, err := s.Create(ctx, "tenant", "user", "", PortalAudience, identity.GroupID, "")
	require.NoError(t, err)
	var stored models.BrowserSession
	require.NoError(t, s.DB.First(&stored, "token_hash = ?", SessionHash(portal)).Error)
	require.NotEqual(t, portal, stored.TokenHash)
	_, err = s.Validate(ctx, portal, AccountAudience, "")
	require.ErrorIs(t, err, ErrSessionInvalid)
	_, err = s.Validate(ctx, portal, PortalAudience, "other-tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
	rotated, err := s.Create(ctx, "tenant", "user", "", PortalAudience, identity.GroupID, portal)
	require.NoError(t, err)
	_, err = s.Validate(ctx, portal, PortalAudience, "tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
	require.NoError(t, s.Revoke(ctx, account, AccountAudience, false))
	_, err = s.Validate(ctx, rotated, PortalAudience, "tenant")
	require.NoError(t, err, "account-only logout must not revoke portal audience")
	require.NoError(t, s.Revoke(ctx, account, AccountAudience, true))
	_, err = s.Validate(ctx, rotated, PortalAudience, "tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestSessionUsesCurrentPermissionsAndExpiry(t *testing.T) {
	s := sessionTestStore(t)
	ctx := t.Context()
	raw, err := s.Create(ctx, "tenant", "user", "", PortalAudience, "", "")
	require.NoError(t, err)
	require.NoError(t, s.DB.Model(&models.User{}).Where("id = ?", "user").Update("role", "client").Error)
	identity, err := s.Validate(ctx, raw, PortalAudience, "tenant")
	require.NoError(t, err)
	require.Equal(t, "client", identity.Role)
	require.NoError(t, s.DB.Model(&models.BrowserSession{}).Where("id = ?", identity.SessionID).Update("idle_expires_at", time.Now().Add(-time.Minute)).Error)
	_, err = s.Validate(ctx, raw, PortalAudience, "tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestIdentitySwitchRevokesPreviousBillingSession(t *testing.T) {
	s := sessionTestStore(t)
	ctx := t.Context()
	account, err := s.Create(ctx, "tenant", "user", "", AccountAudience, "", "")
	require.NoError(t, err)
	identity, err := s.Validate(ctx, account, AccountAudience, "")
	require.NoError(t, err)
	portal, err := s.Create(ctx, "tenant", "user", "", PortalAudience, identity.GroupID, "")
	require.NoError(t, err)
	_, err = s.Create(ctx, "tenant", "", "recipient-link", PortalAudience, "", portal)
	require.NoError(t, err)
	_, err = s.Validate(ctx, account, AccountAudience, "")
	require.ErrorIs(t, err, ErrSessionInvalid)
	_, err = s.Validate(ctx, portal, PortalAudience, "tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestUserRevocationInvalidatesHandoffsAndChallenges(t *testing.T) {
	s := sessionTestStore(t)
	ctx := t.Context()
	raw, err := s.Create(ctx, "tenant", "user", "", PortalAudience, "", "")
	require.NoError(t, err)
	require.NoError(t, s.DB.Create(&models.HostedLoginHandoff{TenantID: "tenant", UserID: "user", CodeHash: "handoff", ExpiresAt: time.Now().Add(time.Minute)}).Error)
	require.NoError(t, s.DB.Create(&models.WebAuthnChallenge{TenantID: "tenant", UserID: "user", SessionData: []byte("{}"), ExpiresAt: time.Now().Add(time.Minute)}).Error)
	require.NoError(t, s.RevokeUser(ctx, "tenant", "user"))
	_, err = s.Validate(ctx, raw, PortalAudience, "tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
	for _, model := range []any{&models.HostedLoginHandoff{}, &models.WebAuthnChallenge{}} {
		var count int64
		require.NoError(t, s.DB.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}
