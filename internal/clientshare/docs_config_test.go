package clientshare

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDocsURLBranding(t *testing.T) {
	for _, tc := range []struct {
		name, mode, override, want string
	}{
		{"default single", "", "", "/docs/"},
		{"single", "single", "", "/docs/"},
		{"hosted", "hosted", "", "/docs/"},
		{"hosted blank override", "hosted", "   ", "/docs/"},
		{"hosted override", "hosted", " https://help.example.com/guide/ ", "https://help.example.com/guide/"},
		{"single override", "single", "https://help.example.com", "https://help.example.com"},
		{"relative override", "single", "/help/", "/help/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := newTestConfig(root)
			cfg.Tenancy.Mode = tc.mode
			cfg.Server.DocsURL = tc.override
			require.NoError(t, cfg.Validate())
			require.Equal(t, tc.want, cfg.DocsURL())
			app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(root))
			require.NoError(t, err)
			req := httptest.NewRequest("GET", "https://frontend.local/api/settings/branding", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			require.Equal(t, tc.want, body["docs_url"])
		})
	}
}

func TestDocsURLValidation(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "data:text/html,test", "//other.example.com", "/\\other.example.com", "/%5cother.example.com", "docs/", "https://", "https://user:pass@example.com", "%"} {
		t.Run(value, func(t *testing.T) {
			cfg := &Config{}
			cfg.Server.DocsURL = value
			require.ErrorContains(t, cfg.Validate(), "server.docs_url")
		})
	}
}
