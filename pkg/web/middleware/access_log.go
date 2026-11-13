package middleware

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type request struct {
	method        string
	url           string
	contentLength int
	hostname      string
	remoteAddress string
	userAgent     string
}

// Access logs retain routing context, not bearer credentials, auth query values
// or invitation fragments. This also applies to redirect destinations.
func accessLogURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid-url]"
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
	u.ForceQuery = false
	for _, prefix := range []string{"/links/", "/api/link/"} {
		if strings.HasPrefix(u.Path, prefix) {
			u.Path, u.RawPath = prefix+"[redacted]", ""
			break
		}
	}
	return u.String()
}

func (r request) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("method", r.method)
	enc.AddString("url", r.url)
	enc.AddInt("bytes", r.contentLength)
	enc.AddString("hostname", r.hostname)
	enc.AddString("remoteAddress", r.remoteAddress)
	enc.AddString("userAgent", r.userAgent)
	return nil
}

type response struct {
	statusCode int
	bytes      int
	handler    string
	location   string
}

func (r response) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt("statusCode", r.statusCode)
	enc.AddInt("bytes", r.bytes)
	enc.AddString("handler", r.handler)
	enc.AddString("location", r.location)
	return nil
}

// NewAccessLogger returns a middleware that logs requests and responses
// using the provided logger.
//
// Pass disableSSRLogs=true to skip logging for SSR requests (useful in development)
func NewAccessLogger(logger *zap.Logger, disableSSRLogs bool, useColor bool) fiber.Handler {
	var (
		once       sync.Once
		errHandler fiber.ErrorHandler
	)

	return func(c fiber.Ctx) error {
		// Set error handler once
		once.Do(func() {
			errHandler = c.App().Config().ErrorHandler
		})

		// create new logger with context
		child := logger
		if child == nil {
			child = utils.Logger(c)
		}
		if requestId := c.GetRespHeader(fiber.HeaderXRequestID); requestId != "" {
			child = child.With(zap.String("requestId", requestId))
		}

		c.Locals(utils.LoggerContextKey, child)
		c.SetContext(context.WithValue(c.Context(), utils.LoggerContextKey, child))

		// serve request and capture timings
		startTime := time.Now()
		chainErr := c.Next()
		if chainErr != nil {
			if err := errHandler(c, chainErr); err != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}
		// note that elapsed time might be incorrect when using response streaming
		// (as the handler will return before the entire response is written)
		elapsedTime := time.Since(startTime)

		LogWebRequest(c, &startTime, &elapsedTime, disableSSRLogs, useColor, chainErr)

		return nil
	}
}

func LogWebRequest(c fiber.Ctx, startTime *time.Time, elapsedTime *time.Duration, disableSSRLogs bool, useColor bool, chainErr error) {
	handlerType := "unknown"
	if handler := fiber.Locals[string](c, "handler"); handler != "" {
		handlerType = handler
	}

	if disableSSRLogs && handlerType == "ssr" {
		return // skip logging for SSR when requested
	}

	req := &request{
		method:        c.Method(),
		url:           accessLogURL(c.OriginalURL()),
		hostname:      c.Hostname(),
		remoteAddress: utils.GetIP(c),
		contentLength: len(c.Request().Body()),
		// User-Agent is untrusted and can otherwise grow every access log entry.
		userAgent: accessLogUserAgent(c.Get(fiber.HeaderUserAgent)),
	}

	// When streaming a response, do not log the response body size.
	// Trying to read the size will cause response flushing to block until
	// the stream is closed, flushing headers+body at the same time.
	sentBytes := -1
	if !c.Response().IsBodyStream() {
		sentBytes = len(c.Response().Body())
	}

	res := &response{
		statusCode: c.Response().StatusCode(),
		bytes:      sentBytes,
		handler:    handlerType,
	}
	if res.statusCode >= 300 && res.statusCode < 400 {
		res.location = accessLogURL(string(c.Response().Header.Peek(fiber.HeaderLocation)))
	}

	fields := []zap.Field{
		zap.Timep("time", startTime),
		zap.Object("req", req),
		zap.Object("res", res),
		zap.Durationp("elapsedTime", elapsedTime),
	}
	// Read locals after downstream tenant resolution/authentication. Never infer
	// an identity from request headers, query parameters or an unvalidated cookie.
	for _, key := range []string{"tenant_id", "user_id"} {
		if id := fiber.Locals[string](c, key); id != "" {
			fields = append(fields, zap.String(key, id))
		}
	}
	if outcome := fiber.Locals[string](c, "auth_outcome"); outcome != "" {
		fields = append(fields, zap.String("auth_outcome", outcome))
	}
	if chainErr != nil {
		fields = append(fields, zap.Error(chainErr))
	}
	path := c.Path()
	isHealth := strings.HasPrefix(path, "/api/ping") ||
		strings.HasSuffix(path, "/healthz")

	child := utils.Logger(c)

	msg := req.method
	if useColor && child.Level().Enabled(zapcore.DebugLevel) {
		switch msg {
		case "OPTIONS":
			msg = color.YellowString(msg)
		case "GET":
			msg = color.GreenString(msg)
		default:
			msg = color.HiRedString(msg)
		}
	}
	msg += " " + "request completed"

	if res.statusCode >= 400 {
		child = child.WithOptions(zap.AddStacktrace(zap.FatalLevel))
		if res.statusCode >= 500 {
			child.Error(msg, fields...)
		} else {
			child.Warn(msg, fields...)
		}
	} else if isHealth {
		// health checks can be very noisy, so log them at debug level
		child.Debug(msg, fields...)
	} else {
		child.Info(msg, fields...)
	}
}

func accessLogUserAgent(value string) string {
	const maxBytes = 1024
	if len(value) > maxBytes {
		return strings.ToValidUTF8(value[:maxBytes], "")
	}
	return strings.ToValidUTF8(value, "")
}
