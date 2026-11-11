package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/links"
	"github.com/stretchr/testify/require"
)

func TestPendingSharingContactsAreListedAndCanBePromotedWithoutChangingIdentity(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	viewer := models.DefaultAccessRoles(env.client.TenantID)[0]
	require.NoError(t, env.db.Create(&viewer).Error)
	app := lifecycleApp(env, nil)
	create := jsonAuthRequest(http.MethodPost, "/api/clients/"+env.client.ID+"/invitations", env.adminToken, map[string]any{"email": "pending-contact@example.test", "role_id": viewer.ID})
	create.Header.Set("Idempotency-Key", "pending-directory-contact")
	resp, err := app.Test(create)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var invitation models.AccessInvitation
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&invitation))
	resp.Body.Close()
	require.NotNil(t, invitation.RecipientUserID)
	for _, token := range []string{env.adminToken, env.managerToken} {
		for _, query := range []string{
			"search=pending-contact&invite_status=pending&role=external",
			"search=pending-contact&client_id=" + env.client.ID,
		} {
			resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/users?"+query, token, nil))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var result paginatedUsersResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			resp.Body.Close()
			require.Len(t, result.Items, 1)
			require.Equal(t, *invitation.RecipientUserID, result.Items[0].ID)
			require.False(t, result.Items[0].InviteAccepted)
			require.Empty(t, result.Items[0].ClientIDs, "pending invitations are not workspace membership")
			require.Equal(t, "external", result.Items[0].AccessProfile.UserType)
		}
	}
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPut, "/api/users/"+*invitation.RecipientUserID, env.adminToken, map[string]any{
		"email": invitation.Email, "name": "New teammate", "user_type": "internal", "organization_role": "member", "workspace_access": []any{},
	}))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	var user models.User
	require.NoError(t, env.db.First(&user, "id = ?", *invitation.RecipientUserID).Error)
	require.Equal(t, "manager", user.Role)
	require.Equal(t, invitation.Email, user.Email)
	profile, err := services.AccessProfileFor(env.db, user)
	require.NoError(t, err)
	require.Equal(t, "internal", profile.UserType)
	require.Equal(t, "member", profile.OrganizationRole)
	d, err := (&services.AccessService{DB: env.db}).Resolve(t.Context(), services.AccessRequest{TenantID: user.TenantID, ClientID: env.client.ID, Actor: services.AccessActor{UserID: user.ID}, Operation: services.AccessRead})
	require.NoError(t, err)
	require.False(t, d.Allowed, "promotion to Member and a pending invitation alone grant no document access")
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPut, "/api/users/"+user.ID, env.adminToken, map[string]any{
		"email": user.Email, "name": user.Name, "user_type": "internal", "organization_role": "administrator", "workspace_access": []any{},
	}))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	require.NoError(t, env.db.First(&user, "id = ?", *invitation.RecipientUserID).Error)
	require.Equal(t, "admin", user.Role)
	require.Equal(t, invitation.Email, user.Email)
	var count int64
	require.NoError(t, env.db.Model(&models.User{}).Where("tenant_id = ? AND LOWER(email) = ?", user.TenantID, strings.ToLower(user.Email)).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestPendingSharingDirectoryDoesNotLeakPrivateScopes(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	service := &services.SharingService{DB: env.db, TokenSecret: "invite-secret"}
	_, err := service.CreateInvitation(t.Context(), services.AccessRequest{TenantID: env.client.TenantID, ClientID: env.otherClient.ID, Actor: services.AccessActor{UserID: env.adminUser.ID}}, services.InvitationInput{Email: "private-pending@example.test", Capabilities: models.AccessCapabilities{CanRead: true}}, "private-pending")
	require.NoError(t, err)
	for _, query := range []string{"search=private-pending", "client_id=" + env.otherClient.ID} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/users?"+query, env.managerToken, nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var result paginatedUsersResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		resp.Body.Close()
		require.Empty(t, result.Items)
	}
}

func TestManagerDirectoryKeepsAcceptedItemOnlyRecipientsWithinScope(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	contact := models.User{TenantID: env.client.TenantID, Email: "item-contact@example.test", Role: "client"}
	require.NoError(t, env.db.Create(&contact).Error)
	grant := models.ItemGrant{TenantID: env.client.TenantID, ClientID: env.client.ID, UserID: contact.ID, FileID: env.rootFolder.ID, CreatedBy: env.adminUser.ID, AccessCapabilities: models.AccessCapabilities{CanRead: true}}
	require.NoError(t, env.db.Create(&grant).Error)
	for _, query := range []string{"search=item-contact", "search=item-contact&client_id=" + env.client.ID} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/users?"+query, env.managerToken, nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var result paginatedUsersResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		resp.Body.Close()
		require.Len(t, result.Items, 1)
		require.Equal(t, contact.ID, result.Items[0].ID)
		require.Empty(t, result.Items[0].ClientIDs)
	}
	require.NoError(t, env.db.Model(&grant).Update("revoked_at", models.Now()).Error)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/users?search=item-contact", env.managerToken, nil))
	require.NoError(t, err)
	var result paginatedUsersResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	resp.Body.Close()
	require.Empty(t, result.Items)
}

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
