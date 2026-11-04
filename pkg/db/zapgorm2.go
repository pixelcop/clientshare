package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/pixelcop/clientshare/pkg/utils"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type Logger struct {
	ZapLogger                 *zap.Logger
	LogLevel                  gormlogger.LogLevel
	SlowThreshold             time.Duration
	SkipCallerLookup          bool
	IgnoreRecordNotFoundError bool
	PrintQuery                bool // When true, print the query to the console (without escapes)
	SingleLine                bool // Keep SQL in a structured field, escaping newlines instead of printing them.
}

func NewZapLogger(zapLogger *zap.Logger, logLevel int) Logger {
	singleLine, _ := strconv.ParseBool(os.Getenv("GORM_LOG_SINGLE_LINE"))
	return Logger{
		ZapLogger:                 zapLogger,
		LogLevel:                  gormlogger.LogLevel(logLevel),
		SlowThreshold:             100 * time.Millisecond,
		SkipCallerLookup:          false,
		IgnoreRecordNotFoundError: false,
		SingleLine:                singleLine,
	}
}

func (l Logger) SetAsDefault() {
	gormlogger.Default = l
}

func (l Logger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	l.LogLevel = level
	return l
}

func (l Logger) Info(ctx context.Context, str string, args ...interface{}) {
	if l.LogLevel < gormlogger.Info {
		return
	}
	l.logger(ctx).Sugar().Debugf(str, args...)
}

func (l Logger) Warn(ctx context.Context, str string, args ...interface{}) {
	if l.LogLevel < gormlogger.Warn {
		return
	}
	l.logger(ctx).Sugar().Warnf(str, args...)
}

func (l Logger) Error(ctx context.Context, str string, args ...interface{}) {
	if l.LogLevel < gormlogger.Error {
		return
	}
	l.logger(ctx).Sugar().Errorf(str, args...)
}

func (l Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= 0 {
		return
	}
	elapsed := time.Since(begin)
	var sql string
	var rows int64
	switch {
	case err != nil && l.LogLevel >= gormlogger.Error && (!l.IgnoreRecordNotFoundError || !errors.Is(err, gorm.ErrRecordNotFound)):
		sql, rows = fc()
		l.logger(ctx).Error("trace", zap.Error(err), zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
	case l.SlowThreshold != 0 && elapsed > l.SlowThreshold && l.LogLevel >= gormlogger.Warn:
		sql, rows = fc()
		l.logger(ctx).Warn("trace", zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
	case l.LogLevel >= gormlogger.Info:
		sql, rows = fc()
		if l.SingleLine {
			l.logger(ctx).Debug("trace", zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
			break
		}
		lsql := strings.ToLower(sql)
		if strings.HasPrefix(lsql, "select") {
			l.logger(ctx).Debug(color.YellowString(sql), zap.Duration("elapsed", elapsed), zap.Int64("rows", rows))
		} else {
			l.logger(ctx).Debug(color.RedString(sql), zap.Duration("elapsed", elapsed), zap.Int64("rows", rows))
		}
	}

	if l.PrintQuery && !l.SingleLine {
		sql, _ := fc()
		fmt.Print("\n\n-----\n", sql, "\n-----\n\n\n")
	}
}

var (
	gormPackage = filepath.Join("gorm.io", "gorm")
)

// create child logger with adjusted caller skip
func (l Logger) logger(ctx context.Context) *zap.Logger {
	var zl *zap.Logger
	if v := ctx.Value(utils.LoggerContextKey); v != nil {
		zl = v.(*zap.Logger)
	} else {
		zl = l.ZapLogger
	}

	for i := 2; i < 15; i++ {
		_, file, _, ok := runtime.Caller(i)
		switch {
		case !ok:
		case strings.HasSuffix(file, "_test.go"):
		case strings.Contains(file, gormPackage):
		default:
			return zl.WithOptions(zap.AddCallerSkip(i))
		}
	}
	return zl
}
