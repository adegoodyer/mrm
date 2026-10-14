// Package logger writes mrm's JSON log to $XDG_STATE_HOME/mrm/mrm.log.
//
// Log startup, runs, and failures with enough context to diagnose them.
// Never log secrets.
package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// L is the global logger; a no-op until Init.
var L = zap.NewNop()

// Init opens the log file. A failure leaves the no-op logger in place: mrm
// works without a log.
func Init(path string, debug bool) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}
	cfg := zap.Config{
		Encoding:         "json",
		Level:            zap.NewAtomicLevelAt(level),
		OutputPaths:      []string{path},
		ErrorOutputPaths: []string{path},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:   "message",
			LevelKey:     "level",
			EncodeLevel:  zapcore.LowercaseLevelEncoder,
			TimeKey:      "timestamp",
			EncodeTime:   zapcore.ISO8601TimeEncoder,
			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,
		},
	}
	if l, err := cfg.Build(); err == nil {
		L = l
	}
}
