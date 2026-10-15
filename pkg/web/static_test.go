package web

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestStaticDevProxyLoopback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Host", r.Host)
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	t.Cleanup(upstream.Close)
	port := upstream.Listener.Addr().(*net.TCPAddr).Port
	app := fiber.New()
	require.NoError(t, SetupStaticFileServing(app, true, port, nil))

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/src/main.ts?t=123", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, upstream.Listener.Addr().String(), response.Header.Get("X-Upstream-Host"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "/src/main.ts?t=123", string(body))
}
