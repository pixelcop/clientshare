package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestItemAccessManagementDoesNotExposeInheritedRostersOrStrongerBearerTokens(t *testing.T) {
	f, service := sharingFixture(t)
	f.grant(t, "folder", "recipient", models.AccessCapabilities{CanRead: true, CanShare: true})
	f.grant(t, "file", "customer", models.AccessCapabilities{CanRead: true})
	for _, mode := range []string{"read", "readwrite"} {
		link := models.SecureLink{TenantID: "tenant", ClientID: "client", FileID: accessPtr("folder"), Token: mode + "-secret", AccessType: mode, ExpiresAt: accessTestNow.Add(time.Hour)}
		require.NoError(t, f.db.Create(&link).Error)
	}
	parentLink := models.SecureLink{TenantID: "tenant", ClientID: "client", Token: "parent-bearer-secret", AccessType: "read", ExpiresAt: accessTestNow.Add(time.Hour)}
	require.NoError(t, f.db.Create(&parentLink).Error)
	view, err := service.ItemAccess(t.Context(), f.req("recipient", "folder", AccessShare))
	require.NoError(t, err)
	require.Len(t, view.Grants, 1)
	require.Equal(t, "recipient", view.Grants[0].UserID)
	require.Equal(t, "recipient@example.test", view.Grants[0].RecipientEmail)
	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "read-secret")
	require.NotContains(t, string(encoded), "readwrite-secret")
	require.NotContains(t, string(encoded), parentLink.Token)
	require.NotContains(t, string(encoded), "member@example.test")
	view, err = service.ItemAccess(t.Context(), f.req("recipient", "file", AccessShare))
	require.NoError(t, err)
	require.True(t, view.InheritsFolderAccess)
	require.False(t, view.InheritsClientAccess)
	require.Len(t, view.Grants, 1)
	require.Equal(t, "customer", view.Grants[0].UserID)
	require.Empty(t, view.Links)
	_, err = service.ItemAccess(t.Context(), f.req("recipient", "sibling", AccessShare))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	req := f.req("admin", "file", AccessShare)
	req.Actor.LinkID = parentLink.ID
	_, err = service.ItemAccess(t.Context(), req)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, f.db.Model(&models.ItemGrant{}).Where("user_id = 'recipient'").Update("revoked_at", accessTestNow).Error)
	_, err = service.ItemAccess(t.Context(), f.req("recipient", "file", AccessShare))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
