package clientshare

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/internal/tenant"
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
			cfg.Server.Docs.URL = tc.override
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
			cfg.Server.Docs.URL = value
			require.ErrorContains(t, cfg.Validate(), "server.docs.url")
			cfg.Server.Docs.URL = ""
			cfg.Server.Docs.SiteLink.URL = value
			require.ErrorContains(t, cfg.Validate(), "server.docs.site_link.url")
		})
	}
}

func TestLoadDocsConfig(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"nested", "  docs:\n    url: https://help.example.com/\n", "https://help.example.com/"},
		{"blank URL", "  docs:\n    url: ''\n", "/docs/"},
		{"site link only", "  docs:\n    site_link:\n      enabled: false\n", "/docs/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(requiredConfigYAML+"server:\n"+tc.yaml), 0600))
			cfg, err := LoadConfig(path)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.DocsURL())
			if tc.name == "site link only" {
				require.NotNil(t, cfg.Server.Docs.SiteLink.Enabled)
				require.False(t, *cfg.Server.Docs.SiteLink.Enabled)
			}
		})
	}
}

func TestDocsSiteConfig(t *testing.T) {
	for _, tc := range []struct {
		name, mode, publicURL, baseURL, url, label, wantURL, wantLabel string
		disabled                                                       bool
	}{
		{name: "single public URL", mode: "single", publicURL: "https://portal.example.com", wantURL: "https://portal.example.com", wantLabel: "Back to portal"},
		{name: "single server fallback", baseURL: "https://fallback.example.com", wantURL: "https://fallback.example.com", wantLabel: "Back to portal"},
		{name: "single origin fallback", wantURL: "/", wantLabel: "Back to portal"},
		{name: "hosted defaults hidden", mode: "hosted", publicURL: "https://tenant.example.com", baseURL: "https://fallback.example.com"},
		{name: "hosted explicit URL", mode: "hosted", url: "https://company.example.com", wantURL: "https://company.example.com", wantLabel: "Website"},
		{name: "single explicit URL and label", mode: "single", publicURL: "https://portal.example.com", url: "https://signup.example.com/", label: "Create a workspace", wantURL: "https://signup.example.com/", wantLabel: "Create a workspace"},
		{name: "disabled automatic", disabled: true, publicURL: "https://portal.example.com"},
		{name: "disabled explicit", mode: "hosted", disabled: true, url: "https://company.example.com"},
		{name: "relative explicit", mode: "hosted", url: "/account", wantURL: "/account", wantLabel: "Website"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := newTestConfig(root)
			cfg.Server.BaseURL = tc.baseURL
			cfg.Tenancy.Mode = tc.mode
			cfg.Server.Docs.SiteLink.URL = tc.url
			cfg.Server.Docs.SiteLink.Label = tc.label
			if tc.disabled {
				enabled := false
				cfg.Server.Docs.SiteLink.Enabled = &enabled
			}
			db := newTestDB(t)
			require.NoError(t, db.Model(&models.TenantSettings{}).Where("tenant_id = ?", tenant.DefaultTenantID).Update("public_base_url", tc.publicURL).Error)
			app, err := NewWebApp(cfg, db, zap.NewNop(), nil, storage.NewLocalStorage(root))
			require.NoError(t, err)
			resp, err := app.Test(httptest.NewRequest("GET", "https://untrusted.example/docs/site-config.json", nil))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode)
			require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
			var body docsSiteConfig
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			if tc.wantURL == "" {
				require.Nil(t, body.SiteLink)
			} else {
				require.Equal(t, &docsSiteLink{URL: tc.wantURL, Label: tc.wantLabel}, body.SiteLink)
			}
		})
	}
}
