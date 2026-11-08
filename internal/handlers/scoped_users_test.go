package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/links"
	"github.com/stretchr/testify/require"
)

func TestManagerRosterScopeAndNoStaffEscalation(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	for _, role := range []string{"admin", "manager"} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/users", env.managerToken, map[string]any{"email": role + "-escalation@example.test", "role": role}))
		require.NoError(t, err)
		require.Equal(t, 403, resp.StatusCode)
		resp.Body.Close()
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/users", env.managerToken, map[string]any{"email": "unassigned@example.test", "role": "client", "client_ids": []string{env.otherClient.ID}}))
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	var count int64
	require.NoError(t, env.db.Model(&models.User{}).Where("email = ?", "unassigned@example.test").Count(&count).Error)
	require.Zero(t, count)
	for _, route := range []struct{ method, path string }{
		{http.MethodPut, "/api/users/" + env.clientUser.ID},
		{http.MethodDelete, "/api/users/" + env.clientUser.ID},
		{http.MethodPost, "/api/users/" + env.clientUser.ID + "/generate-access-link"},
		{http.MethodPost, "/api/users/" + env.managerUser.ID + "/assign-client"},
	} {
		resp, err := env.app.Test(jsonAuthRequest(route.method, route.path, env.managerToken, map[string]any{"role": "admin", "password": "takeover", "client_id": env.otherClient.ID}))
		require.NoError(t, err)
		require.Equal(t, 403, resp.StatusCode)
		resp.Body.Close()
	}
	// A roster member may independently belong to a private client. That
	// relationship must be absent both from DTOs and client-name searches.
	require.NoError(t, env.db.Create(&models.UserClient{TenantID: env.client.TenantID, UserID: env.otherClientUser.ID, ClientID: env.client.ID}).Error)
	require.NoError(t, env.db.Model(env.otherClient).Update("name", "HiddenCompanyScopeOnly").Error)
	for _, search := range []string{env.otherClientUser.Email, "HiddenCompanyScopeOnly"} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/users?search="+url.QueryEscape(search), env.managerToken, nil))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		var result paginatedUsersResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		resp.Body.Close()
		if search == "HiddenCompanyScopeOnly" {
			require.Empty(t, result.Items)
			require.Zero(t, result.Pagination.TotalItems)
		} else {
			require.Len(t, result.Items, 1)
			require.Equal(t, []string{env.client.ID}, result.Items[0].ClientIDs)
		}
	}
}

func TestAssignmentUpdatePreservesProvenanceAndRemovesRecipientGrantsOnly(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	require.NoError(t, env.db.Model(&models.UserClient{}).Where("user_id = ?", env.clientUser.ID).Updates(map[string]any{"capabilities_set": true, "can_read": true, "access_source": "independent", "invited_by": nil}).Error)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodPut, "/api/users/"+env.clientUser.ID, env.adminToken, map[string]any{"client_ids": []string{env.client.ID}}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	var membership models.UserClient
	require.NoError(t, env.db.First(&membership, "user_id = ?", env.clientUser.ID).Error)
	require.True(t, membership.CapabilitiesSet)
	require.True(t, membership.CanRead)
	require.False(t, membership.CanShare)
	require.Equal(t, "independent", membership.AccessSource)
	require.Nil(t, membership.InvitedBy)
	grant := models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: env.rootFolder.ID, UserID: env.clientUser.ID, CreatedBy: env.adminUser.ID, AccessCapabilities: models.AccessCapabilities{CanRead: true}}
	onward := models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: env.rootFolder.ID, UserID: env.otherClientUser.ID, CreatedBy: env.clientUser.ID, AccessCapabilities: models.AccessCapabilities{CanRead: true}}
	for _, g := range []*models.ItemGrant{&grant, &onward} {
		require.NoError(t, env.db.Create(g).Error)
	}
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, CreatedBy: env.clientUser.ID, Token: "surviving-bearer", AccessType: "read", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, env.db.Create(&link).Error)
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPut, "/api/users/"+env.clientUser.ID, env.adminToken, map[string]any{"client_ids": []string{env.otherClient.ID}}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	require.NoError(t, env.db.First(&grant, "id = ?", grant.ID).Error)
	require.NotNil(t, grant.RevokedAt)
	require.NoError(t, env.db.First(&onward, "id = ?", onward.ID).Error)
	require.Nil(t, onward.RevokedAt)
	require.NoError(t, env.db.First(&link, "id = ?", link.ID).Error)
	require.Nil(t, link.RevokedAt)
}

func TestBearerRegistrationCannotMintNamedSharingAuthority(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	token, err := links.GenerateSecureToken(env.signingKey)
	require.NoError(t, err)
	link := models.SecureLink{TenantID: env.client.TenantID, ClientID: env.client.ID, Token: token, AccessType: "readwrite", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, env.db.Create(&link).Error)
	resp, err := env.app.Test(jsonRequest(http.MethodPost, "/api/auth/register", map[string]any{"token": token, "email": "registered@example.test", "name": "Registered", "password": "password123"}))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	resp.Body.Close()
	var user models.User
	require.NoError(t, env.db.First(&user, "email = ?", "registered@example.test").Error)
	d, err := (&services.AccessService{DB: env.db}).Resolve(t.Context(), services.AccessRequest{TenantID: env.client.TenantID, ClientID: env.client.ID, Actor: services.AccessActor{UserID: user.ID}, Operation: services.AccessShare})
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.True(t, d.Capabilities.CanRead)
	require.True(t, d.Capabilities.CanUpload)
	require.False(t, d.Capabilities.CanShare)
}
