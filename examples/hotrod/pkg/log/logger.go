package log

import "github.com/uber-go/zap"

// Logger is a simplified abstraction of the zap.Logger
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	With(fields ...zap.Field) Logger
}

// logger delegates all calls to the underlying zap.Logger
type logger struct {
	logger zap.Logger
}

// Info logs an info msg with fields
func (l logger) Info(msg string, fields ...zap.Field) {
	l.logger.Info(msg, fields...)
}

// Error logs an error msg with fields
func (l logger) Error(msg string, fields ...zap.Field) {
	l.logger.Error(msg, fields...)
}

// Fatal logs a fatal error msg with fields
func (l logger) Fatal(msg string, fields ...zap.Field) {
	l.logger.Fatal(msg, fields...)
}

// With creates a child logger, and optionally adds some context fields to that logger.
func (l logger) With(fields ...zap.Field) Logger {
	return logger{logger: l.logger.With(fields...)}
}
