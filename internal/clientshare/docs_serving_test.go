package clientshare

import (
	"io"
	"io/fs"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/internal/static"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewWebAppServesEmbeddedDocs(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		dev        bool
	}{
		{"single production", "single", false},
		{"single development", "single", true},
		{"hosted production", "hosted", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := newTestConfig(root)
			cfg.Tenancy.Mode = tc.mode
			cfg.DevMode = tc.dev
			app, err := NewWebApp(cfg, newTestDB(t), zap.NewNop(), nil, storage.NewLocalStorage(root))
			require.NoError(t, err)
			for _, page := range []string{"/docs/", "/docs/files/upload.html"} {
				resp, err := app.Test(httptest.NewRequest("GET", page, nil))
				require.NoError(t, err)
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				require.NoError(t, err)
				require.Equal(t, 200, resp.StatusCode, "run make build-guide-embedded before testing")
				require.Contains(t, string(body), "ClientShare")
				require.Contains(t, string(body), "/docs/assets/")
				if page == "/docs/files/upload.html" {
					require.Contains(t, string(body), "/docs/screenshots/upload-and-files.png")
				}
			}
			docsFS, err := static.GetEmbeddedGuideFS(nil)
			require.NoError(t, err)
			index, err := fs.ReadFile(docsFS, "index.html")
			require.NoError(t, err)
			assetMatch := regexp.MustCompile(`src="/docs/(assets/[^\"]+\.js)"`).FindSubmatch(index)
			require.Len(t, assetMatch, 2)
			for _, asset := range []string{string(assetMatch[1]), "screenshots/recipient-upload.png"} {
				resp, err := app.Test(httptest.NewRequest("GET", "/docs/"+asset, nil))
				require.NoError(t, err)
				resp.Body.Close()
				require.Equal(t, 200, resp.StatusCode)
			}
		})
	}
}
