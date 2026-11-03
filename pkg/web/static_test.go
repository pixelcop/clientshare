package web

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

func TestStaticDevProxyClosedIdleConnections(t *testing.T) {
	const connections = 12 // More than fasthttp's five retry attempts.
	warm := make(chan struct{}, connections)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warm" {
			warm <- struct{}{}
			<-release
		}
		_, _ = io.WriteString(w, "frontend")
	}))
	t.Cleanup(upstream.Close)
	app := CreateApp(32, zap.NewNop(), false)
	require.NoError(t, SetupStaticFileServing(app, true, upstream.Listener.Addr().(*net.TCPAddr).Port, nil))
	results := make(chan error, connections)
	for range connections {
		go func() {
			res, err := app.Test(httptest.NewRequest(http.MethodGet, "/warm", nil), fiber.TestConfig{Timeout: 10 * time.Second})
			if err == nil {
				defer res.Body.Close()
				if res.StatusCode != http.StatusOK {
					err = fmt.Errorf("warm request status: %d", res.StatusCode)
				}
			}
			results <- err
		}()
	}
	// Release handlers even on failure, before upstream.Close waits for them.
	func() {
		defer close(release)
		for range connections {
			select {
			case <-warm:
			case <-time.After(5 * time.Second):
				t.Fatal("upstream connections did not warm up")
			}
		}
	}()
	for range connections {
		require.NoError(t, <-results)
	}
	upstream.CloseClientConnections()
	for range connections {
		res, err := app.Test(httptest.NewRequest(http.MethodGet, "/refresh", nil))
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, "frontend", string(body))
	}
}

func TestStaticDevProxyUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	app := CreateApp(32, zap.NewNop(), false)
	require.NoError(t, SetupStaticFileServing(app, true, port, nil))
	res, err := app.Test(httptest.NewRequest(http.MethodGet, "/src/main.ts", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	require.Contains(t, string(body), "Vite dev server")
	require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
}

func TestDevProxyError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"connection closed", io.EOF, http.StatusBadGateway},
		{"timeout", &net.DNSError{IsTimeout: true}, http.StatusGatewayTimeout},
		{"wrapped timeout", fmt.Errorf("upstream: %w", &net.DNSError{IsTimeout: true}), http.StatusGatewayTimeout},
		{"other", errors.New("upstream failed"), http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			devProxyError(w, httptest.NewRequest(http.MethodGet, "/", nil), tc.err)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.NotContains(t, w.Body.String(), tc.err.Error())
		})
	}
}

func TestStaticDevProxyWebSocketTunnel(t *testing.T) {
	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Sec-WebSocket-Protocol") != "vite-hmr" || r.URL.RequestURI() != "/?token=test" {
			upstreamResult <- fmt.Errorf("unexpected upgrade request: %s %v", r.URL, r.Header)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstreamResult <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\nSec-WebSocket-Protocol: vite-hmr\r\n\r\n")
		if err == nil {
			err = rw.Flush()
		}
		// Receive a masked client text frame, then send an unmasked server frame.
		frame := make([]byte, 8)
		if err == nil {
			_, err = io.ReadFull(rw, frame)
		}
		if err == nil && string(frame) != string([]byte{0x81, 0x82, 1, 2, 3, 4, 'h' ^ 1, 'i' ^ 2}) {
			err = fmt.Errorf("unexpected client frame: %x", frame)
		}
		if err == nil {
			_, err = conn.Write([]byte{0x81, 2, 'h', 'i'})
		}
		upstreamResult <- err
	}))
	t.Cleanup(upstream.Close)
	app := CreateApp(32, zap.NewNop(), false)
	require.NoError(t, SetupStaticFileServing(app, true, upstream.Listener.Addr().(*net.TCPAddr).Port, nil))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverDone := make(chan error, 1)
	go func() { serverDone <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { require.NoError(t, app.Shutdown()); require.NoError(t, <-serverDone) })
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = fmt.Fprintf(conn, "GET /?token=test HTTP/1.1\r\nHost: preview.example\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Protocol: vite-hmr\r\n\r\n")
	require.NoError(t, err)
	rw := bufio.NewReader(conn)
	res, err := http.ReadResponse(rw, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, res.StatusCode)
	require.Equal(t, "vite-hmr", res.Header.Get("Sec-WebSocket-Protocol"))
	_, err = conn.Write([]byte{0x81, 0x82, 1, 2, 3, 4, 'h' ^ 1, 'i' ^ 2})
	require.NoError(t, err)
	frame := make([]byte, 4)
	_, err = io.ReadFull(rw, frame)
	require.NoError(t, err)
	require.Equal(t, []byte{0x81, 2, 'h', 'i'}, frame)
	require.NoError(t, <-upstreamResult)
}
