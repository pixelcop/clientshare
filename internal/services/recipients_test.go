package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecipientsRespectVisibilityAndNeverGrantAccess(t *testing.T) {
	f, _ := sharingFixture(t)
	s := &RecipientService{DB: f.db}
	for _, user := range []models.User{
		{ID: "other-person", TenantID: "tenant", Email: "other@example.test", Name: "Another", Role: "client"},
		{ID: "foreign-person", TenantID: "foreign", Email: "secret@example.test", Role: "client"},
	} {
		require.NoError(t, f.db.Create(&user).Error)
	}
	for _, membership := range []models.UserClient{
		{TenantID: "tenant", ClientID: "other-client", UserID: "other-person"},
		{TenantID: "tenant", ClientID: "other-client", UserID: "member"},
		{TenantID: "foreign", ClientID: "foreign-client", UserID: "foreign-person"},
	} {
		require.NoError(t, f.db.Create(&membership).Error)
	}
	f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	req := f.req("manager", "", AccessContext)
	contacts, err := s.Search(t.Context(), req, "", true)
	require.NoError(t, err)
	ids := []string{}
	for _, contact := range contacts {
		ids = append(ids, contact.ID)
		for _, client := range contact.Clients {
			require.Equal(t, "client", client.ID, "hidden associations must be redacted")
		}
		if contact.ID == "recipient" {
			require.True(t, contact.CurrentClient)
			require.False(t, contact.WorkspaceAccess, "item access is not whole-client access")
		}
	}
	require.Contains(t, ids, "recipient")
	require.NotContains(t, ids, "other-person")
	require.NotContains(t, ids, "foreign-person")
	req.Actor.UserID = "admin"
	contacts, err = s.Search(t.Context(), req, "OTHER@", true)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Equal(t, "other-person", contacts[0].ID)
	require.False(t, contacts[0].CurrentClient)
	require.False(t, contacts[0].WorkspaceAccess)
	var count int64
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = ? AND client_id = ?", "other-person", "client").Count(&count).Error)
	require.Zero(t, count, "search must not add a membership")
	contacts, err = s.Search(t.Context(), req, "OTHER@", false)
	require.NoError(t, err)
	require.Empty(t, contacts)
	for _, actor := range []string{"member", "customer", "recipient", "missing", "foreign-person"} {
		req.Actor.UserID = actor
		_, err := s.Search(t.Context(), req, "", true)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	req.Actor = AccessActor{UserID: "admin", LinkID: "some-link"}
	_, err = s.Search(t.Context(), req, "", true)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	req.Actor = AccessActor{UserID: "manager"}
	req.ClientID = "other-client"
	_, err = s.Search(t.Context(), req, "", true)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, f.db.Where("tenant_id = ? AND user_id = ?", "tenant", "manager").Delete(&models.UserClient{}).Error)
	f.grant(t, "file", "manager", models.AccessCapabilities{CanRead: true, CanShare: true})
	req.ClientID = "client"
	_, err = s.Search(t.Context(), req, "", false)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "item-only staff cannot discover the client directory")
}

func TestRecipientsSearchIsBoundedLiteralAndDeduplicated(t *testing.T) {
	f, _ := sharingFixture(t)
	s := &RecipientService{DB: f.db}
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", "member").Update("name", "100%_ Real").Error)
	// Overlapping sources still produce a single identity and client association.
	f.grant(t, "file", "member", models.AccessCapabilities{CanRead: true})
	require.NoError(t, f.db.Create(&models.AccessInvitation{TenantID: "tenant", ClientID: "client", RecipientUserID: accessPtr("member"), Email: "member@example.test", TokenHash: "test", CreatedBy: "admin", ExpiresAt: time.Now().Add(time.Hour)}).Error)
	req := f.req("admin", "", AccessContext)
	contacts, err := s.Search(t.Context(), req, "%_", false)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Equal(t, "member", contacts[0].ID)
	require.True(t, contacts[0].WorkspaceAccess)
	require.Len(t, contacts[0].Clients, 1)
	for i := range 25 {
		require.NoError(t, f.db.Create(&models.User{
			ID: fmt.Sprintf("extra-%02d", i), TenantID: "tenant",
			Email: fmt.Sprintf("extra-%02d@example.test", i), Role: "client",
		}).Error)
	}
	contacts, err = s.Search(t.Context(), req, "", true)
	require.NoError(t, err)
	require.True(t, contacts[0].CurrentClient, "current-client contacts rank first")
	require.Len(t, contacts, 20)
}
