package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestAccessRoleAndIdentityHTTPContract(t *testing.T) {
	env := setupFileTestEnv(t)
	roleBody := map[string]any{"name": "Outside reviewer", "audience": "both", "allow_workspace": true, "allow_folder": true, "allow_file": true, "can_read": true}
	response, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/access-roles", env.otherToken, roleBody))
	require.NoError(t, err)
	require.Equal(t, 403, response.StatusCode)
	response.Body.Close()
	response, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/access-roles", env.adminToken, roleBody))
	require.NoError(t, err)
	require.Equal(t, 201, response.StatusCode)
	var role models.AccessRole
	require.NoError(t, json.NewDecoder(response.Body).Decode(&role))
	response.Body.Close()
	require.NotEmpty(t, role.ID)
	require.True(t, role.CanRead)
	require.False(t, role.CanInviteExternal)
	create := map[string]any{"email": "new-member@example.test", "name": "New Member", "user_type": "internal", "organization_role": "member", "workspace_access": []any{}, "group_ids": []string{}}
	response, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, create))
	require.NoError(t, err)
	require.Equal(t, 201, response.StatusCode)
	var user models.User
	require.NoError(t, json.NewDecoder(response.Body).Decode(&user))
	response.Body.Close()
	var profile models.AccessProfile
	require.NoError(t, env.db.First(&profile, "user_id = ?", user.ID).Error)
	require.False(t, profile.CanManageUsers || profile.AllWorkspaces)
	var count int64
	require.NoError(t, env.db.Model(&models.UserClient{}).Where("user_id = ?", user.ID).Count(&count).Error)
	require.Zero(t, count)
	// External identity alone is valid and cannot receive organization powers.
	create["email"], create["user_type"], create["organization_role"] = "external@example.test", "external", "none"
	response, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, create))
	require.NoError(t, err)
	require.Equal(t, 201, response.StatusCode)
	response.Body.Close()
	create["email"], create["organization_role"] = "invalid@example.test", "administrator"
	response, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, create))
	require.NoError(t, err)
	require.Equal(t, 400, response.StatusCode)
	response.Body.Close()
}
