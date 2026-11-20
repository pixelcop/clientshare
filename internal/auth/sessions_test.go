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
	require.NoError(t, db.AutoMigrate(&models.BrowserSession{}, &models.User{}, &models.TenantEntitlement{}, &models.SecureLink{}, &models.HostedLoginHandoff{}, &models.WebAuthnChallenge{}, &models.Client{}, &models.File{}))
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

func TestSessionExpiryUsesInstantsAcrossOffsets(t *testing.T) {
	s := sessionTestStore(t)
	now := time.Now().UTC()
	expired := now.Add(-time.Hour).In(time.FixedZone("east", 3*3600))
	active := now.Add(time.Hour).In(time.FixedZone("west", -3*3600))
	for _, tt := range []struct {
		name          string
		expires, idle time.Time
		valid         bool
	}{
		{"active", active, active, true},
		{"absolute expiry", expired, active, false},
		{"idle expiry", active, expired, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := s.Create(t.Context(), "tenant", "user", "", PortalAudience, "", "")
			require.NoError(t, err)
			require.NoError(t, s.DB.Model(&models.BrowserSession{}).Where("token_hash = ?", SessionHash(raw)).
				Updates(map[string]any{"expires_at": tt.expires, "idle_expires_at": tt.idle}).Error)
			_, err = s.Validate(t.Context(), raw, PortalAudience, "tenant")
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrSessionInvalid)
			}
		})
	}
}

func TestScopedLinkSessionsRevalidateTargetAndAncestry(t *testing.T) {
	s := sessionTestStore(t)
	target := "item"
	root := "root"
	require.NoError(t, s.DB.Create(&models.Client{ID: "client", TenantID: "tenant", RootFolderID: &root}).Error)
	require.NoError(t, s.DB.Create(&models.File{ID: root, TenantID: "tenant", ClientID: "client", Type: "folder"}).Error)
	require.NoError(t, s.DB.Create(&models.File{ID: target, TenantID: "tenant", ClientID: "client", Type: "file", FolderID: &root}).Error)
	link := models.SecureLink{ID: "scoped", TenantID: "tenant", ClientID: "client", FileID: &target, Token: "scoped", AccessType: "read", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, s.DB.Create(&link).Error)
	raw, err := s.Create(t.Context(), "tenant", "", link.ID, PortalAudience, "", "")
	require.NoError(t, err)
	identity, err := s.Validate(t.Context(), raw, PortalAudience, "tenant")
	require.NoError(t, err)
	require.Equal(t, target, identity.FileID)
	require.Equal(t, "link", identity.Role)
	require.Empty(t, identity.UserID)
	require.NoError(t, s.DB.Model(&models.File{}).Where("id = ?", root).Update("deleted_at", time.Now()).Error)
	_, err = s.Validate(t.Context(), raw, PortalAudience, "tenant")
	require.ErrorIs(t, err, ErrSessionInvalid)
}
