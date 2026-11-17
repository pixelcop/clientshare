package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/pixelcop/clientshare/internal/services"
	"github.com/stretchr/testify/require"
)

func TestRecipientDirectoryHTTPIsStaffOnlyAndMinimal(t *testing.T) {
	env := setupFileTestEnv(t)
	url := "/api/recipients?client_id=" + env.client.ID
	for _, token := range []string{env.clientToken, env.otherToken, env.linkToken} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, url, token, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
	}
	resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, url, env.adminToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	for _, field := range []string{"password", "role", "token", "tenant_id", "capabilities"} {
		require.NotContains(t, string(data), `"`+field+`"`)
	}
	var contacts []services.RecipientContact
	require.NoError(t, json.Unmarshal(data, &contacts))
	require.NotEmpty(t, contacts)
	resp, err = env.app.Test(jsonAuthRequest(http.MethodGet, url+"&scope=invalid", env.adminToken, nil))
	require.NoError(t, err)
	require.Equal(t, 400, resp.StatusCode)
	resp.Body.Close()
}
