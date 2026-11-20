package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestScopedLinkCreationCeilingsReplayAndNonrecursiveRevocation(t *testing.T) {
	f, service := sharingFixture(t)
	service.LinkSigningKey = "test-link-signing-key"
	f.grant(t, "folder", "recipient", models.AccessCapabilities{CanRead: true, CanShare: true})
	input := LinkInput{AccessType: "read", ExpiryDays: 7}
	req := f.req("recipient", "folder", AccessShare)
	link, err := service.CreateLink(t.Context(), req, input, "scoped-link-key")
	require.NoError(t, err)
	require.Equal(t, "folder", *link.FileID)
	replay, err := service.CreateLink(t.Context(), req, input, "scoped-link-key")
	require.NoError(t, err)
	require.Equal(t, link.ID, replay.ID)
	_, err = service.CreateLink(t.Context(), req, LinkInput{AccessType: "write", ExpiryDays: 7}, "cannot-escalate")
	require.ErrorIs(t, err, ErrSharingCapabilities)
	_, err = service.CreateLink(t.Context(), f.req("recipient", "", AccessShare), input, "cannot-promote-client")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = service.CreateLink(t.Context(), f.req("recipient", "sibling", AccessShare), input, "cannot-promote-sibling")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = service.CreateLink(t.Context(), req, LinkInput{AccessType: "view", ExpiryDays: 7}, "scoped-link-key")
	require.ErrorIs(t, err, ErrLinkRequestConflict)
	mail := models.QueuedEmail{TenantID: "tenant", SecureLinkID: &link.ID, Recipient: "contact@example.test"}
	require.NoError(t, f.db.Create(&mail).Error)
	require.NoError(t, f.db.Model(&models.ItemGrant{}).Where("user_id = 'recipient'").Update("revoked_at", accessTestNow).Error)
	_, err = service.LinkForDelivery(t.Context(), req, link.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.ErrorIs(t, service.RevokeLink(t.Context(), req, link.ID), gorm.ErrRecordNotFound)
	require.NoError(t, f.db.First(link, "id = ?", link.ID).Error)
	require.Nil(t, link.RevokedAt, "onward bearer access survives the creator's loss of access")
	require.NoError(t, service.RevokeLink(t.Context(), f.req("admin", "", AccessShare), link.ID))
	require.NoError(t, f.db.First(&mail, "id = ?", mail.ID).Error)
	require.NotNil(t, mail.CanceledAt)
	replay, err = service.CreateLink(t.Context(), f.req("admin", "folder", AccessShare), input, "admin-link-key")
	require.NoError(t, err)
	require.NoError(t, service.RevokeLink(t.Context(), f.req("admin", "", AccessShare), replay.ID))
	retired, err := service.CreateLink(t.Context(), f.req("admin", "folder", AccessShare), input, "admin-link-key")
	require.NoError(t, err)
	require.Equal(t, replay.ID, retired.ID)
	require.NotNil(t, retired.RevokedAt)
	var audits []models.AuditEvent
	require.NoError(t, f.db.Find(&audits).Error)
	encoded, err := json.Marshal(audits)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), link.Token)
	require.NotContains(t, string(encoded), replay.Token)
}

func TestFileLinksRejectUploadModesAndBearerActorsCannotCreateLinks(t *testing.T) {
	f, service := sharingFixture(t)
	service.LinkSigningKey = "test-link-signing-key"
	for _, mode := range []string{"write", "readwrite"} {
		_, err := service.CreateLink(t.Context(), f.req("admin", "file", AccessShare), LinkInput{AccessType: mode, ExpiryDays: 7}, "file-"+mode)
		require.Error(t, err)
	}
	link, err := service.CreateLink(t.Context(), f.req("admin", "file", AccessShare), LinkInput{AccessType: "view", ExpiryDays: 7}, "file-view")
	require.NoError(t, err)
	req := f.req("admin", "file", AccessShare)
	req.Actor.LinkID = link.ID
	_, err = service.CreateLink(t.Context(), req, LinkInput{AccessType: "read", ExpiryDays: 7}, "bearer-cannot-share")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	service.Now = func() time.Time { return link.ExpiresAt }
	_, err = service.LinkForDelivery(t.Context(), f.req("admin", "file", AccessShare), link.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestScopedLinkListIncludesOnlyRecordedOpensForItsTenantAndLinks(t *testing.T) {
	f, service := sharingFixture(t)
	require.NoError(t, f.db.AutoMigrate(&models.FileActivity{}))
	service.LinkSigningKey = "test-link-signing-key"
	req := f.req("admin", "folder", AccessShare)
	used, err := service.CreateLink(t.Context(), req, LinkInput{AccessType: "read", ExpiryDays: 7}, "used")
	require.NoError(t, err)
	unused, err := service.CreateLink(t.Context(), req, LinkInput{AccessType: "read", ExpiryDays: 7}, "unused")
	require.NoError(t, err)
	earlier := accessTestNow.Add(-2 * time.Hour).In(time.FixedZone("east", 3*3600))
	latest := accessTestNow.Add(-time.Hour).In(time.FixedZone("west", -3*3600))
	for _, activity := range []models.FileActivity{
		{TenantID: "tenant", ClientID: used.ClientID, SecureLinkID: &used.ID, Action: "view_link", CreatedAt: earlier},
		{TenantID: "tenant", ClientID: used.ClientID, SecureLinkID: &used.ID, Action: "view_link", CreatedAt: latest},
		{TenantID: "other-tenant", ClientID: used.ClientID, SecureLinkID: &used.ID, Action: "view_link", CreatedAt: accessTestNow},
		{TenantID: "tenant", ClientID: unused.ClientID, SecureLinkID: &unused.ID, Action: "email_link", CreatedAt: accessTestNow},
	} {
		require.NoError(t, f.db.Create(&activity).Error)
	}
	listed, err := service.LinksAtScope(t.Context(), req)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	for _, link := range listed {
		if link.ID == used.ID {
			require.NotNil(t, link.LastAccessed)
			require.True(t, latest.Equal(*link.LastAccessed))
		} else {
			require.Nil(t, link.LastAccessed, "emailing a link is not opening it")
		}
	}
}
