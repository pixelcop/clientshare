package services

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMemberRemovalRevokesRecipientAccessNotOnwardShares(t *testing.T) {
	f, service := sharingFixture(t)
	owner := "member"
	require.NoError(t, f.db.Create(&models.UserClient{TenantID: "tenant", ClientID: "client", UserID: "recipient", CapabilitiesSet: true, AccessCapabilities: models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true}, AccessSource: "invitation", InvitedBy: &owner}).Error)
	grant := f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	require.NoError(t, f.db.Create(&models.User{ID: "peer", TenantID: "tenant", Role: "client", Email: "peer@example.test"}).Error)
	onward, err := service.CreateGrant(t.Context(), f.req("recipient", "file", AccessShare), GrantInput{UserID: "peer", Capabilities: models.AccessCapabilities{CanRead: true}})
	require.NoError(t, err)
	pending, err := service.CreateInvitation(t.Context(), f.req("member", "file", AccessShare), InvitationInput{Email: "recipient@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "recipient-pending-key")
	require.NoError(t, err)
	onwardPending, err := service.CreateInvitation(t.Context(), f.req("recipient", "file", AccessShare), InvitationInput{Email: "peer@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "onward-pending-key")
	require.NoError(t, err)
	link := models.SecureLink{TenantID: "tenant", ClientID: "client", Token: "retained-bearer-secret", CreatedBy: "recipient", AccessType: "read", ExpiresAt: accessTestNow.Add(time.Hour)}
	require.NoError(t, f.db.Create(&link).Error)
	mail := models.QueuedEmail{TenantID: "tenant", Recipient: pending.Email, AccessInvitationID: &pending.ID}
	onwardMail := models.QueuedEmail{TenantID: "tenant", Recipient: onwardPending.Email, AccessInvitationID: &onwardPending.ID}
	require.NoError(t, f.db.Create(&mail).Error)
	require.NoError(t, f.db.Create(&onwardMail).Error)
	require.NoError(t, service.RemoveMember(t.Context(), f.req("member", "", AccessInviteClient), "recipient", false))
	require.NoError(t, f.db.First(&mail, "id = ?", mail.ID).Error)
	require.NotNil(t, mail.CanceledAt)
	require.NoError(t, f.db.First(&onwardMail, "id = ?", onwardMail.ID).Error)
	require.Nil(t, onwardMail.CanceledAt)
	var count int64
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = 'recipient'").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, f.db.First(grant, "id = ?", grant.ID).Error)
	require.NotNil(t, grant.RevokedAt)
	require.Greater(t, grant.Version, uint64(1))
	require.NoError(t, f.db.First(pending, "id = ?", pending.ID).Error)
	require.NotNil(t, pending.RevokedAt)
	_, err = service.AcceptInvitation(t.Context(), "tenant", "recipient", service.invitationToken(pending.ID))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, f.db.First(onward, "id = ?", onward.ID).Error)
	require.Nil(t, onward.RevokedAt)
	require.NoError(t, f.db.First(onwardPending, "id = ?", onwardPending.ID).Error)
	require.Nil(t, onwardPending.RevokedAt)
	require.NoError(t, f.db.First(&link, "id = ?", link.ID).Error)
	require.Nil(t, link.RevokedAt)
	f.check(t, f.req("recipient", "file", AccessRead), false)
	f.check(t, f.req("peer", "file", AccessRead), true)
}

func TestMemberOwnershipStaffAndBearerRevokeBoundaries(t *testing.T) {
	f, service := sharingFixture(t)
	require.ErrorIs(t, service.RemoveMember(t.Context(), f.req("member", "", AccessInviteClient), "customer", false), gorm.ErrRecordNotFound, "independent members cannot be taken over")
	require.ErrorIs(t, service.RemoveMember(t.Context(), f.req("member", "", AccessInviteClient), "manager", false), gorm.ErrRecordNotFound, "client users never manage staff")
	for _, target := range []*string{nil, accessPtr("file"), accessPtr("folder")} {
		link := models.SecureLink{TenantID: "tenant", ClientID: "client", FileID: target, Token: modelsTokenForTest(target), AccessType: "read", ExpiresAt: accessTestNow.Add(time.Hour)}
		require.NoError(t, f.db.Create(&link).Error)
	}
	require.NoError(t, service.RemoveMember(t.Context(), f.req("manager", "", AccessInviteClient), "customer", true))
	var links []models.SecureLink
	require.NoError(t, f.db.Find(&links).Error)
	for _, link := range links {
		require.NotNil(t, link.RevokedAt)
	}
	var audit []models.AuditEvent
	require.NoError(t, f.db.Find(&audit).Error)
	for _, row := range audit {
		for _, link := range links {
			require.NotContains(t, row.Payload, link.Token)
		}
	}
}

func modelsTokenForTest(target *string) string {
	if target == nil {
		return "client-bearer-secret"
	}
	return *target + "-bearer-secret"
}
