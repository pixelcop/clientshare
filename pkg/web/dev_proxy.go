package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"go.uber.org/zap"
)

func newDevProxy(port int) fiber.Handler {
	// net/http watches idle sockets and retries safe requests on fresh
	// connections. The fasthttp balancer can exhaust its retry budget on
	// stale pooled sockets after Vite closes a batch of idle connections.
	transport := &http.Transport{
		// No environment proxy: this upstream must always stay on loopback.
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		IdleConnTimeout:       2 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		ResponseHeaderTimeout: 30 * time.Second, // Allow initial compilation.
		DisableCompression:    true,             // Preserve Vite's response encoding.
	}
	upstream := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.SetXForwarded()
		},
		ErrorHandler: devProxyError,
	}
	// ReverseProxy handles the HTTP upgrade and bidirectional WebSocket
	// tunnel; Fiber's adapter supports hijacking the downstream connection.
	return adaptor.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The adapter's context is a pooled fasthttp RequestCtx. Transport's
		// background goroutines must not consult its Locals after this handler
		// returns (or while Fiber logs an upgraded request).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func devProxyError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadGateway
	message := "development frontend unavailable; check that the Vite dev server is running"
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		status = http.StatusGatewayTimeout
		message = "development frontend timed out; Vite may still be compiling"
	}
	zap.L().Warn("development frontend proxy failed", zap.String("path", r.URL.Path), zap.Error(err))
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, message, status)
}
