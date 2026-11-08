package services

import (
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClientAccessRosterPreservesOwnInviteeAndWholeClientBoundaries(t *testing.T) {
	f, service := sharingFixture(t)
	owner := "member"
	require.NoError(t, f.db.Create(&models.UserClient{TenantID: "tenant", ClientID: "client", UserID: "recipient", AccessSource: "invitation", InvitedBy: &owner, CapabilitiesSet: true, AccessCapabilities: models.AccessCapabilities{CanRead: true}}).Error)
	// An invitation owned by the same actor in another client is not a match.
	require.NoError(t, f.db.Create(&models.UserClient{TenantID: "tenant", ClientID: "other-client", UserID: "admin", AccessSource: "invitation", InvitedBy: &owner}).Error)
	view, err := service.ClientAccess(t.Context(), f.req("member", "", AccessShare))
	require.NoError(t, err)
	require.False(t, view.CanManage)
	require.Len(t, view.People, 2)
	people := map[string]ClientMemberAccess{}
	for _, person := range view.People {
		people[person.UserID] = person
	}
	require.False(t, people["member"].CanRemove)
	require.True(t, people["member"].Capabilities.CanShare)
	require.True(t, people["recipient"].CanRemove)
	require.False(t, people["recipient"].Capabilities.CanUpload)
	require.NotContains(t, people, "admin")
	require.NotContains(t, people, "customer")
	view, err = service.ClientAccess(t.Context(), f.req("manager", "", AccessShare))
	require.NoError(t, err)
	require.True(t, view.CanManage)
	require.Len(t, view.People, 4)
	f.grant(t, "root", "recipient", models.AccessCapabilities{CanRead: true, CanShare: true})
	require.NoError(t, f.db.Where("user_id = 'recipient'").Delete(&models.UserClient{}).Error)
	_, err = service.ClientAccess(t.Context(), f.req("recipient", "", AccessShare))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "even a root grant never grants roster access")
}
