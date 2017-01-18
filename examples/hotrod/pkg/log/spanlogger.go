package log

import (
	"github.com/opentracing/opentracing-go"
	tag "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/uber-go/zap"
)

type spanLogger struct {
	logger zap.Logger
	span   opentracing.Span
}

func (sl spanLogger) Info(msg string, fields ...zap.Field) {
	sl.logToSpan("info", msg, fields...)
	sl.logger.Info(msg, fields...)
}

func (sl spanLogger) Error(msg string, fields ...zap.Field) {
	sl.logToSpan("error", msg, fields...)
	sl.logger.Error(msg, fields...)
}

func (sl spanLogger) Fatal(msg string, fields ...zap.Field) {
	sl.logToSpan("fatal", msg, fields...)
	tag.Error.Set(sl.span, true)
	sl.logger.Fatal(msg, fields...)
}

// With creates a child logger, and optionally adds some context fields to that logger.
func (sl spanLogger) With(fields ...zap.Field) Logger {
	return spanLogger{logger: sl.logger.With(fields...), span: sl.span}
}

func (sl spanLogger) logToSpan(level string, msg string, fields ...zap.Field) {
	// TODO rather than always converting the fields, we could wrap them into a lazy logger
	fa := fieldAdapter(make([]log.Field, 0, 2+len(fields)))
	fa = append(fa, log.String("event", msg))
	fa = append(fa, log.String("level", level))
	for _, field := range fields {
		field.AddTo(&fa)
	}
	sl.span.LogFields(fa...)
}

type fieldAdapter []log.Field

func (fa *fieldAdapter) AddBool(key string, value bool) {
	*fa = append(*fa, log.Bool(key, value))
}

func (fa *fieldAdapter) AddFloat64(key string, value float64) {
	*fa = append(*fa, log.Float64(key, value))
}

func (fa *fieldAdapter) AddInt(key string, value int) {
	*fa = append(*fa, log.Int(key, value))
}

func (fa *fieldAdapter) AddInt64(key string, value int64) {
	*fa = append(*fa, log.Int64(key, value))
}

func (fa *fieldAdapter) AddUint(key string, value uint) {
	*fa = append(*fa, log.Uint64(key, uint64(value)))
}

func (fa *fieldAdapter) AddUint64(key string, value uint64) {
	*fa = append(*fa, log.Uint64(key, value))
}

func (fa *fieldAdapter) AddUintptr(key string, value uintptr) {
	// TODO *fa = append(*fa, log.Float64(key, value))
}

func (fa *fieldAdapter) AddMarshaler(key string, marshaler zap.LogMarshaler) error {
	// TODO *fa = append(*fa, log.Float64(key, value))
	return nil
}

func (fa *fieldAdapter) AddObject(key string, value interface{}) error {
	*fa = append(*fa, log.Object(key, value))
	return nil
}

func (fa *fieldAdapter) AddString(key, value string) {
	if key != "" && value != "" {
		*fa = append(*fa, log.String(key, value))
	}
}
