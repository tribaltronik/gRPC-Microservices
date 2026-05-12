package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a zap.Logger configured by the given level string ("debug", "info", "warn", "error").
// In production, this should be called after config is loaded to set the right level.
func New(level string) (*zap.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = zapcore.InfoLevel
	}

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(lvl),
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return config.Build()
}
