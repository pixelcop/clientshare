package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestNewZapLoggerSingleLine(t *testing.T) {
	for _, value := range []string{"", "false", "true", "1", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("GORM_LOG_SINGLE_LINE", value)
			l := NewZapLogger(zap.NewNop(), int(gormlogger.Info))
			want := value == "true" || value == "1"
			if l.SingleLine != want {
				t.Fatalf("SingleLine = %v, want %v", l.SingleLine, want)
			}
			l.PrintQuery = true
			copy := l.LogMode(gormlogger.Error).(Logger)
			if copy.SingleLine != want || !copy.PrintQuery || copy.LogLevel != gormlogger.Error {
				t.Fatalf("LogMode lost logger options: %+v", copy)
			}
		})
	}
}

func TestSingleLineTrace(t *testing.T) {
	// Keep SQL intact, including whitespace inside literals. The structured
	// encoder, not SQL rewriting, escapes newlines onto a single physical line.
	const query = "SELECT 'two  spaces\nand a newline'\r\nFROM users\nWHERE id = 1"
	for _, encoding := range []string{"console", "json"} {
		for _, tc := range []struct {
			name  string
			begin time.Time
			err   error
			level zapcore.Level
		}{
			{"normal", time.Now(), nil, zap.DebugLevel},
			{"slow", time.Now().Add(-time.Second), nil, zap.WarnLevel},
			{"error", time.Now(), errors.New("query failed"), zap.ErrorLevel},
		} {
			t.Run(encoding+"/"+tc.name, func(t *testing.T) {
				t.Setenv("GORM_LOG_SINGLE_LINE", "true")
				var output strings.Builder
				cfg := zap.NewDevelopmentEncoderConfig()
				var encoder zapcore.Encoder = zapcore.NewConsoleEncoder(cfg)
				if encoding == "json" {
					encoder = zapcore.NewJSONEncoder(cfg)
				}
				observed, entries := observer.New(zap.DebugLevel)
				core := zapcore.NewTee(observed, zapcore.NewCore(encoder, zapcore.AddSync(&output), zap.DebugLevel))
				l := NewZapLogger(zap.New(core), int(gormlogger.Info))
				l.PrintQuery = true // Must not bypass structured output in this mode.
				l.Trace(context.Background(), tc.begin, func() (string, int64) { return query, 2 }, tc.err)
				if entries.Len() != 1 {
					t.Fatalf("got %d entries, want 1", entries.Len())
				}
				entry := entries.All()[0]
				if entry.Level != tc.level || entry.Message != "trace" || entry.ContextMap()["sql"] != query || entry.ContextMap()["rows"] != int64(2) {
					t.Fatalf("unexpected entry: %+v", entry)
				}
				if strings.Count(output.String(), "\n") != 1 || strings.Contains(output.String(), "\r") {
					t.Fatalf("output spans multiple lines: %q", output.String())
				}
			})
		}
	}
}

func TestLegacyTracePreservesMultilineMessage(t *testing.T) {
	t.Setenv("GORM_LOG_SINGLE_LINE", "false")
	core, entries := observer.New(zap.DebugLevel)
	l := NewZapLogger(zap.New(core), int(gormlogger.Info))
	const query = "SELECT *\nFROM users"
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return query, 1 }, nil)
	if entries.Len() != 1 || !strings.Contains(entries.All()[0].Message, query) {
		t.Fatal("legacy SQL message was changed")
	}
	if _, ok := entries.All()[0].ContextMap()["sql"]; ok {
		t.Fatal("legacy trace unexpectedly moved SQL to a field")
	}
}

func TestSingleLineTraceIgnoresRecordNotFound(t *testing.T) {
	t.Setenv("GORM_LOG_SINGLE_LINE", "true")
	core, entries := observer.New(zap.DebugLevel)
	l := NewZapLogger(zap.New(core), int(gormlogger.Error))
	l.IgnoreRecordNotFoundError = true
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		t.Fatal("ignored query evaluated")
		return "", 0
	}, gorm.ErrRecordNotFound)
	if entries.Len() != 0 {
		t.Fatal("ignored error was logged")
	}
}
