// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type spanLogger struct {
	logger     *zap.Logger
	span       trace.Span
	spanFields []zapcore.Field
}

func (sl spanLogger) Debug(msg string, fields ...zapcore.Field) {
	sl.logToSpan("debug", msg, fields...)
	sl.logger.Debug(msg, append(sl.spanFields, fields...)...)
}

func (sl spanLogger) Info(msg string, fields ...zapcore.Field) {
	sl.logToSpan("info", msg, fields...)
	sl.logger.Info(msg, append(sl.spanFields, fields...)...)
}

func (sl spanLogger) Error(msg string, fields ...zapcore.Field) {
	sl.logToSpan("error", msg, fields...)
	sl.logger.Error(msg, append(sl.spanFields, fields...)...)
}

func (sl spanLogger) Fatal(msg string, fields ...zapcore.Field) {
	sl.logToSpan("fatal", msg, fields...)
	sl.span.SetStatus(codes.Error, msg)
	sl.logger.Fatal(msg, append(sl.spanFields, fields...)...)
}

// With creates a child logger, and optionally adds some context fields to that logger.
func (sl spanLogger) With(fields ...zapcore.Field) Logger {
	return spanLogger{logger: sl.logger.With(fields...), span: sl.span, spanFields: sl.spanFields}
}

func (sl spanLogger) logToSpan(level, msg string, fields ...zapcore.Field) {
	fields = append(fields, zap.String("level", level))
	sl.span.AddEvent(
		msg,
		trace.WithAttributes(logFieldsToOTelAttrs(fields)...),
	)
}

func logFieldsToOTelAttrs(fields []zapcore.Field) []attribute.KeyValue {
	encoder := &bridgeFieldEncoder{}
	for _, field := range fields {
		field.AddTo(encoder)
	}
	return encoder.pairs
}

type bridgeFieldEncoder struct {
	pairs []attribute.KeyValue
}

func (e *bridgeFieldEncoder) AddArray(key string, marshaler zapcore.ArrayMarshaler) error {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(marshaler)))
	return nil
}

func (e *bridgeFieldEncoder) AddObject(key string, marshaler zapcore.ObjectMarshaler) error {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(marshaler)))
	return nil
}

func (e *bridgeFieldEncoder) AddBinary(key string, value []byte) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (e *bridgeFieldEncoder) AddByteString(key string, value []byte) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (e *bridgeFieldEncoder) AddBool(key string, value bool) {
	e.pairs = append(e.pairs, attribute.Bool(key, value))
}

func (e *bridgeFieldEncoder) AddComplex128(key string, value complex128) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (e *bridgeFieldEncoder) AddComplex64(key string, value complex64) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (e *bridgeFieldEncoder) AddDuration(key string, value time.Duration) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (e *bridgeFieldEncoder) AddFloat64(key string, value float64) {
	e.pairs = append(e.pairs, attribute.Float64(key, value))
}

func (e *bridgeFieldEncoder) AddFloat32(key string, value float32) {
	e.pairs = append(e.pairs, attribute.Float64(key, float64(value)))
}

func (e *bridgeFieldEncoder) AddInt(key string, value int) {
	e.pairs = append(e.pairs, attribute.Int(key, value))
}

func (e *bridgeFieldEncoder) AddInt64(key string, value int64) {
	e.pairs = append(e.pairs, attribute.Int64(key, value))
}

func (e *bridgeFieldEncoder) AddInt32(key string, value int32) {
	e.pairs = append(e.pairs, attribute.Int64(key, int64(value)))
}

func (e *bridgeFieldEncoder) AddInt16(key string, value int16) {
	e.pairs = append(e.pairs, attribute.Int64(key, int64(value)))
}

func (e *bridgeFieldEncoder) AddInt8(key string, value int8) {
	e.pairs = append(e.pairs, attribute.Int64(key, int64(value)))
}

func (e *bridgeFieldEncoder) AddString(key, value string) {
	e.pairs = append(e.pairs, attribute.String(key, value))
}

func (e *bridgeFieldEncoder) AddTime(key string, value time.Time) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (e *bridgeFieldEncoder) AddUint(key string, value uint) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprintf("%d", value)))
}

func (e *bridgeFieldEncoder) AddUint64(key string, value uint64) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprintf("%d", value)))
}

func (e *bridgeFieldEncoder) AddUint32(key string, value uint32) {
	e.pairs = append(e.pairs, attribute.Int64(key, int64(value)))
}

func (e *bridgeFieldEncoder) AddUint16(key string, value uint16) {
	e.pairs = append(e.pairs, attribute.Int64(key, int64(value)))
}

func (e *bridgeFieldEncoder) AddUint8(key string, value uint8) {
	e.pairs = append(e.pairs, attribute.Int64(key, int64(value)))
}

func (e *bridgeFieldEncoder) AddUintptr(key string, value uintptr) {
	e.pairs = append(e.pairs, attribute.String(key, fmt.Sprint(value)))
}

func (*bridgeFieldEncoder) AddReflected(string /* key */, any /* value */) error { return nil }
func (*bridgeFieldEncoder) OpenNamespace(string /* key */)                       {}
