package utils

import (
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ContextKey string

const LoggerContextKey = ContextKey("logger")

var (
	logErrStack     *zap.Logger
	runtimeLogLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
)

// Logger returns a request-scoped logger, if available
func Logger(c fiber.Ctx) *zap.Logger {
	if c == nil {
		return zap.L()
	}
	l := c.Locals(LoggerContextKey)
	if l == nil {
		return zap.L()
	}
	return l.(*zap.Logger)
}

// LogErrStack returns a logger with stack trace enabled only at error level (good for printing simple warnings)
func LogErrStack() *zap.Logger {
	if logErrStack == nil {
		logErrStack = zap.L().WithOptions(zap.AddStacktrace(zap.ErrorLevel))
	}
	return logErrStack
}

func CurrentLogLevel() string {
	return runtimeLogLevel.Level().String()
}

func SupportedLogLevels() []string {
	return []string{"debug", "info", "warn", "error", "dpanic", "panic", "fatal"}
}

func SetLogLevel(level string) error {
	parsedLevel, err := zapcore.ParseLevel(strings.TrimSpace(level))
	if err != nil {
		return err
	}
	runtimeLogLevel.SetLevel(parsedLevel)
	return nil
}

func InitLogger() {
	var logger *zap.Logger
	isDev, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	if isDev {
		lc := zap.NewDevelopmentConfig()
		lc.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		runtimeLogLevel.SetLevel(initialLogLevel(lc.Level.Level()))
		lc.Level = runtimeLogLevel
		logger, _ = lc.Build()
	} else {
		color.NoColor = true
		lc := zap.NewProductionConfig()
		runtimeLogLevel.SetLevel(initialLogLevel(zap.InfoLevel))
		lc.Level = runtimeLogLevel
		logger, _ = lc.Build()
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()
}

func initialLogLevel(defaultLevel zapcore.Level) zapcore.Level {
	configuredLevel := strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	if configuredLevel == "" {
		return defaultLevel
	}

	parsedLevel, err := zapcore.ParseLevel(configuredLevel)
	if err != nil {
		return defaultLevel
	}

	return parsedLevel
}
