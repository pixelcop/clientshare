package services

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestItemInvitationAcceptanceDoesNotCreateMembershipOrRestoreRevokedAccess(t *testing.T) {
	f, service := sharingFixture(t)
	input := InvitationInput{Email: "Recipient <RECIPIENT@example.test>", Capabilities: models.AccessCapabilities{CanRead: true}, GrantExpiresAt: accessPtr(accessTestNow.Add(time.Hour))}
	invite, err := service.CreateInvitation(t.Context(), f.req("admin", "file", AccessShare), input, "create-invite-key")
	require.NoError(t, err)
	require.Equal(t, "recipient@example.test", invite.Email)
	replay, err := service.CreateInvitation(t.Context(), f.req("admin", "file", AccessShare), input, "create-invite-key")
	require.NoError(t, err)
	require.Equal(t, invite.ID, replay.ID)
	input.Capabilities.CanShare = true
	_, err = service.CreateInvitation(t.Context(), f.req("admin", "file", AccessShare), input, "create-invite-key")
	require.ErrorIs(t, err, ErrSharingConflict)
	token := service.invitationToken(invite.ID)
	_, err = service.AcceptInvitation(t.Context(), "tenant", "member", token)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	accepted, err := service.AcceptInvitation(t.Context(), "tenant", "recipient", token)
	require.NoError(t, err)
	require.NotNil(t, accepted.AcceptedAt)
	var count int64
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = 'recipient'").Count(&count).Error)
	require.Zero(t, count)
	var grant models.ItemGrant
	require.NoError(t, f.db.First(&grant, "user_id = 'recipient' AND file_id = 'file'").Error)
	require.True(t, grant.CanRead)
	require.False(t, grant.CanShare)
	require.Equal(t, invite.CreatedBy, grant.CreatedBy)
	require.True(t, grant.ExpiresAt.Equal(*invite.GrantExpiresAt))
	_, err = service.AcceptInvitation(t.Context(), "tenant", "recipient", token)
	require.NoError(t, err)
	_, err = service.UpdateGrant(t.Context(), f.req("admin", "file", AccessShare), grant.ID, grant.Version, GrantInput{}, true)
	require.NoError(t, err)
	_, err = service.AcceptInvitation(t.Context(), "tenant", "recipient", token)
	require.NoError(t, err, "duplicate acceptance is a no-op, never reactivation")
	f.check(t, f.req("recipient", "file", AccessRead), false)
}

func TestInvitationIdentityTenantLifetimeAndTargetChecks(t *testing.T) {
	for _, change := range []string{"foreign-tenant", "changed-email", "revoked", "expired", "deleted-ancestor"} {
		t.Run(change, func(t *testing.T) {
			f, service := sharingFixture(t)
			invite, err := service.CreateInvitation(t.Context(), f.req("admin", "file", AccessShare), InvitationInput{Email: "recipient@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "scope-check-key")
			require.NoError(t, err)
			tenant := "tenant"
			switch change {
			case "foreign-tenant":
				tenant = "foreign"
			case "changed-email":
				require.NoError(t, f.db.Model(&models.User{}).Where("id = 'recipient'").Update("email", "different@example.test").Error)
			case "revoked":
				require.NoError(t, f.db.Model(invite).Update("revoked_at", accessTestNow).Error)
			case "expired":
				service.Now = func() time.Time { return invite.ExpiresAt }
			case "deleted-ancestor":
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'folder'").Update("deleted_at", accessTestNow).Error)
			}
			_, err = service.AcceptInvitation(t.Context(), tenant, "recipient", service.invitationToken(invite.ID))
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			var count int64
			require.NoError(t, f.db.Model(&models.ItemGrant{}).Where("user_id = 'recipient'").Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestInvitationSurvivesCreatorLossAndCannotTakeOverIndependentMembership(t *testing.T) {
	f, service := sharingFixture(t)
	invite, err := service.CreateInvitation(t.Context(), f.req("member", "folder", AccessShare), InvitationInput{Email: "recipient@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "onward-invite-key")
	require.NoError(t, err)
	require.NoError(t, f.db.Where("user_id = 'member'").Delete(&models.UserClient{}).Error)
	_, _, err = service.InvitationForDelivery(t.Context(), f.req("member", "folder", AccessShare), invite.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "creator no longer manages the surviving invitation")
	_, err = service.AcceptInvitation(t.Context(), "tenant", "recipient", service.invitationToken(invite.ID))
	require.NoError(t, err, "already-issued onward intent survives creator access loss")
	f.check(t, f.req("recipient", "file", AccessRead), true)
	membershipInvite, err := service.CreateInvitation(t.Context(), f.req("admin", "", AccessInviteClient), InvitationInput{Email: "customer@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "independent-invite-key")
	require.NoError(t, err)
	_, err = service.AcceptInvitation(t.Context(), "tenant", "customer", service.invitationToken(membershipInvite.ID))
	require.NoError(t, err)
	var membership models.UserClient
	require.NoError(t, f.db.First(&membership, "user_id = 'customer'").Error)
	require.False(t, membership.CapabilitiesSet)
	require.Equal(t, "independent", membership.AccessSource)
	require.Nil(t, membership.InvitedBy)
	_, err = service.CreateInvitation(t.Context(), f.req("admin", "", AccessInviteClient), InvitationInput{Email: "manager@example.test", Capabilities: models.AccessCapabilities{CanRead: true, CanShare: true}}, "no-staff-assignment-key")
	require.ErrorIs(t, err, ErrSharingInput)
}
