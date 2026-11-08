package services

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func sharingFixture(t *testing.T) (accessFixture, *SharingService) {
	t.Helper()
	f := newAccessFixture(t)
	require.NoError(t, f.db.AutoMigrate(&models.AuditEvent{}, &models.AccessInvitation{}, &models.InviteToken{}, &models.PasskeyCredential{}, &models.QueuedEmail{}))
	return f, &SharingService{DB: f.db, TokenSecret: "test-only-invitation-secret", Now: func() time.Time { return accessTestNow }}
}

func TestGrantMutationsAreExplicitVersionedAndAudited(t *testing.T) {
	f, service := sharingFixture(t)
	req := f.req("admin", "file", AccessShare)
	input := GrantInput{UserID: "recipient", Capabilities: models.AccessCapabilities{CanRead: true}}
	grant, err := service.CreateGrant(t.Context(), req, input)
	require.NoError(t, err)
	require.EqualValues(t, 1, grant.Version)
	firstID := grant.ID
	repeated, err := service.CreateGrant(t.Context(), req, input)
	require.NoError(t, err)
	require.Equal(t, firstID, repeated.ID)
	var count int64
	require.NoError(t, f.db.Model(&models.AuditEvent{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	input.Capabilities.CanShare = true
	_, err = service.CreateGrant(t.Context(), req, input)
	require.ErrorIs(t, err, ErrSharingConflict)
	grant, err = service.UpdateGrant(t.Context(), req, grant.ID, 1, input, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, grant.Version)
	_, err = service.UpdateGrant(t.Context(), req, grant.ID, 1, GrantInput{}, true)
	require.ErrorIs(t, err, ErrSharingConflict)
	invite := models.AccessInvitation{TenantID: "tenant", ClientID: "client", FileID: accessPtr("file"), Email: "recipient@example.test", TokenHash: "not-a-bearer-token", CreatedBy: "admin", ExpiresAt: accessTestNow.Add(time.Hour)}
	require.NoError(t, f.db.Create(&invite).Error)
	grant, err = service.UpdateGrant(t.Context(), req, grant.ID, 2, GrantInput{}, true)
	require.NoError(t, err)
	require.EqualValues(t, 3, grant.Version)
	require.NotNil(t, grant.RevokedAt)
	require.NoError(t, f.db.First(&invite, "id = ?", invite.ID).Error)
	require.NotNil(t, invite.RevokedAt, "pending invitations cannot restore a revoked grant")
	_, err = service.CreateGrant(t.Context(), req, input)
	require.ErrorIs(t, err, ErrSharingConflict)
	grant, err = service.UpdateGrant(t.Context(), req, grant.ID, 3, input, false)
	require.NoError(t, err)
	require.Nil(t, grant.RevokedAt)
	require.EqualValues(t, 4, grant.Version)
	var audit []models.AuditEvent
	require.NoError(t, f.db.Order("created_at ASC").Find(&audit).Error)
	require.Len(t, audit, 5)
	for _, event := range audit {
		require.NotContains(t, event.Payload, invite.TokenHash)
	}
}

func TestGrantAuthorityCeilingsTargetsAndOnwardIndependence(t *testing.T) {
	f, service := sharingFixture(t)
	read := GrantInput{UserID: "recipient", Capabilities: models.AccessCapabilities{CanRead: true, CanShare: true}}
	grant, err := service.CreateGrant(t.Context(), f.req("admin", "file", AccessShare), read)
	require.NoError(t, err)
	for _, req := range []AccessRequest{
		f.req("recipient", "folder", AccessShare),
		f.req("recipient", "sibling", AccessShare),
		{TenantID: "foreign", ClientID: "client", FileID: "file", Actor: AccessActor{UserID: "admin"}},
		{TenantID: "tenant", ClientID: "client", FileID: "file", Actor: AccessActor{UserID: "admin", LinkID: "link"}},
	} {
		_, err := service.CreateGrant(t.Context(), req, GrantInput{UserID: "customer", Capabilities: models.AccessCapabilities{CanRead: true}})
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	_, err = service.CreateGrant(t.Context(), f.req("member", "file", AccessShare), GrantInput{UserID: "customer", Capabilities: models.AccessCapabilities{CanDelete: true}})
	require.ErrorIs(t, err, ErrSharingCapabilities)
	_, err = service.CreateGrant(t.Context(), f.req("admin", "file", AccessShare), GrantInput{UserID: "customer", Capabilities: models.AccessCapabilities{CanUpload: true}})
	require.Error(t, err, "file targets never permit upload")
	require.NoError(t, f.db.Create(&models.User{ID: "peer", TenantID: "tenant", Email: "peer@example.test", Role: "client"}).Error)
	onward, err := service.CreateGrant(t.Context(), f.req("recipient", "file", AccessShare), GrantInput{UserID: "peer", Capabilities: models.AccessCapabilities{CanRead: true}})
	require.NoError(t, err)
	_, err = service.UpdateGrant(t.Context(), f.req("admin", "file", AccessShare), grant.ID, grant.Version, GrantInput{}, true)
	require.NoError(t, err)
	f.check(t, f.req("peer", "file", AccessRead), true)
	_, err = service.UpdateGrant(t.Context(), f.req("recipient", "file", AccessShare), onward.ID, onward.Version, GrantInput{}, true)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "creator loses management when their own access ends")
	require.NoError(t, f.db.Model(&models.File{}).Where("id = 'folder'").Update("deleted_at", accessTestNow).Error)
	_, err = service.CreateGrant(t.Context(), f.req("admin", "file", AccessShare), read)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
