package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInvitationDeliveryCreatesOnlyAnUnassignedIdentityAndReusesItsProof(t *testing.T) {
	f, service := sharingFixture(t)
	req := f.req("admin", "file", AccessShare)
	invite, err := service.CreateInvitation(t.Context(), req, InvitationInput{Email: "brand-new@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "new-identity-invite-key")
	require.NoError(t, err)
	first, err := service.PrepareInvitationDelivery(t.Context(), req, invite.ID, 72*time.Hour)
	require.NoError(t, err)
	require.True(t, first.NeedsIdentity)
	require.Equal(t, "client", first.User.Role)
	require.Empty(t, first.User.PasswordHash)
	require.NotNil(t, first.IdentityInviteID)
	encoded, err := json.Marshal(first)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), first.RawToken, "credentials must never be serializable in inviter responses")
	second, err := service.PrepareInvitationDelivery(t.Context(), req, invite.ID, 72*time.Hour)
	require.NoError(t, err)
	require.Equal(t, first.User.ID, second.User.ID)
	require.Equal(t, *first.IdentityInviteID, *second.IdentityInviteID)
	require.Equal(t, first.RawToken, second.RawToken)
	var count int64
	for _, model := range []any{&models.UserClient{}, &models.ItemGrant{}} {
		require.NoError(t, f.db.Model(model).Where("user_id = ?", first.User.ID).Count(&count).Error)
		require.Zero(t, count, "preparing identity delivery cannot establish access")
	}
	_, err = service.AcceptIdentityInvitation(t.Context(), "tenant", first.User.ID, *first.IdentityInviteID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a proof must be verified/claimed before acceptance")
	require.NoError(t, f.db.Model(&models.InviteToken{}).Where("id = ?", *first.IdentityInviteID).Update("used_at", accessTestNow).Error)
	_, err = service.AcceptIdentityInvitation(t.Context(), "tenant", first.User.ID, *first.IdentityInviteID)
	require.NoError(t, err)
	f.check(t, f.req(first.User.ID, "file", AccessRead), true)
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = ?", first.User.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestInvitationDeliveryDoesNotResetOrReassignExistingAccounts(t *testing.T) {
	f, service := sharingFixture(t)
	require.NoError(t, f.db.Model(&models.User{}).Where("id = 'recipient'").Update("password_hash", "existing-credential").Error)
	req := f.req("admin", "folder", AccessShare)
	invite, err := service.CreateInvitation(t.Context(), req, InvitationInput{Email: "recipient@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "reuse-account-key")
	require.NoError(t, err)
	delivery, err := service.PrepareInvitationDelivery(t.Context(), req, invite.ID, 72*time.Hour)
	require.NoError(t, err)
	require.False(t, delivery.NeedsIdentity)
	require.Nil(t, delivery.IdentityInviteID)
	require.Equal(t, service.invitationToken(invite.ID), delivery.RawToken)
	var user models.User
	require.NoError(t, f.db.First(&user, "id = 'recipient'").Error)
	require.Equal(t, "existing-credential", user.PasswordHash)
	require.Equal(t, "client", user.Role)
	var count int64
	require.NoError(t, f.db.Model(&models.InviteToken{}).Where("user_id = 'recipient'").Count(&count).Error)
	require.Zero(t, count)
}
