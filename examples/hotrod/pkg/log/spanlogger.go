// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	tag "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type otelSpanLogger struct {
	logger     *zap.Logger
	span       trace.Span
	spanFields []zapcore.Field
}

func (sl otelSpanLogger) Debug(msg string, fields ...zapcore.Field) {
	sl.logToSpan("Debug", msg, fields...)
	sl.logger.Debug(msg, append(sl.spanFields, fields...)...)
}

func (sl otelSpanLogger) Info(msg string, fields ...zapcore.Field) {
	sl.logToSpan("info", msg, fields...)
	sl.logger.Info(msg, append(sl.spanFields, fields...)...)
}

func (sl otelSpanLogger) Error(msg string, fields ...zapcore.Field) {
	sl.logToSpan("error", msg, fields...)
	sl.logger.Error(msg, append(sl.spanFields, fields...)...)
}

func (sl otelSpanLogger) Fatal(msg string, fields ...zapcore.Field) {
	sl.logToSpan("fatal", msg, fields...)
	sl.span.SetStatus(codes.Error, msg)
	sl.logger.Fatal(msg, append(sl.spanFields, fields...)...)
}

// With creates a child logger, and optionally adds some context fields to that logger.
func (sl otelSpanLogger) With(fields ...zapcore.Field) Logger {
	return otelSpanLogger{logger: sl.logger.With(fields...), span: sl.span, spanFields: sl.spanFields}
}

// See: https://github.com/open-telemetry/opentelemetry-go/blob/main/bridge/opentracing/bridge.go#L168
func (sl otelSpanLogger) logToSpan(level string, msg string, fields ...zapcore.Field) {
	sl.span.AddEvent(
		msg,
		trace.WithAttributes(otLogFieldsToOTelAttrs(fields)...),
	)
}

func otLogFieldsToOTelAttrs(fields []zapcore.Field) []attribute.KeyValue {
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
	e.pairs = append(e.pairs, attribute.Key(key).String(fmt.Sprint(marshaler)))
	return nil
}

func (e *bridgeFieldEncoder) AddObject(key string, marshaler zapcore.ObjectMarshaler) error {
	// TODO implement me
	panic("AddObject implement me")
}

func (e *bridgeFieldEncoder) AddBinary(key string, value []byte) {
	// TODO implement me
	panic("AddBinary implement me")
}

func (e *bridgeFieldEncoder) AddByteString(key string, value []byte) {
	// TODO implement me
	panic("AddByteString implement me")
}

func (e *bridgeFieldEncoder) AddBool(key string, value bool) {
	// TODO implement me
	panic("AddBool implement me")
}

func (e *bridgeFieldEncoder) AddComplex128(key string, value complex128) {
	// TODO implement me
	panic("AddComplex128 implement me")
}

func (e *bridgeFieldEncoder) AddComplex64(key string, value complex64) {
	// TODO implement me
	panic("AddComplex64 implement me")
}

func (e *bridgeFieldEncoder) AddDuration(key string, value time.Duration) {
	// TODO implement me
	panic("AddDuration implement me")
}

func (e *bridgeFieldEncoder) AddFloat64(key string, value float64) {
	// TODO implement me
	panic("AddFloat64 implement me")
}

func (e *bridgeFieldEncoder) AddFloat32(key string, value float32) {
	// TODO implement me
	panic("AddFloat32 implement me")
}

func (e *bridgeFieldEncoder) AddInt(key string, value int) {
	// TODO implement me
	panic("AddInt implement me")
}

func (e *bridgeFieldEncoder) AddInt64(key string, value int64) {
	e.pairs = append(e.pairs, attribute.Key(key).Int64(value))
}

func (e *bridgeFieldEncoder) AddInt32(key string, value int32) {
	// TODO implement me
	panic("AddInt32 implement me")
}

func (e *bridgeFieldEncoder) AddInt16(key string, value int16) {
	// TODO implement me
	panic("AddInt16 implement me")
}

func (e *bridgeFieldEncoder) AddInt8(key string, value int8) {
	// TODO implement me
	panic("AddInt8 implement me")
}

func (e *bridgeFieldEncoder) AddString(key, value string) {
	e.pairs = append(e.pairs, attribute.Key(key).String(value))
}

func (e *bridgeFieldEncoder) AddTime(key string, value time.Time) {
	// TODO implement me
	panic("AddTime implement me")
}

func (e *bridgeFieldEncoder) AddUint(key string, value uint) {
	// TODO implement me
	panic("AddUint implement me")
}

func (e *bridgeFieldEncoder) AddUint64(key string, value uint64) {
	// TODO implement me
	panic("AddUint64 implement me")
}

func (e *bridgeFieldEncoder) AddUint32(key string, value uint32) {
	// TODO implement me
	panic("AddUint32 implement me")
}

func (e *bridgeFieldEncoder) AddUint16(key string, value uint16) {
	// TODO implement me
	panic("AddUint16 implement me")
}

func (e *bridgeFieldEncoder) AddUint8(key string, value uint8) {
	// TODO implement me
	panic("AddUint8 implement me")
}

func (e *bridgeFieldEncoder) AddUintptr(key string, value uintptr) {
	// TODO implement me
	panic("AddUintptr implement me")
}

func (e *bridgeFieldEncoder) AddReflected(key string, value interface{}) error {
	// TODO implement me
	panic("AddReflected implement me")
}

func (e *bridgeFieldEncoder) OpenNamespace(key string) {
	// TODO implement me
	panic("OpenNamespace implement me")
}

// otTagToOTelAttr converts given key-value into attribute.KeyValue.
// Note that some conversions are not obvious:
// - int -> int64
// - uint -> string
// - int32 -> int64
// - uint32 -> int64
// - uint64 -> string
// - float32 -> float64
func otTagToOTelAttr(k string, v interface{}) attribute.KeyValue {
	key := otTagToOTelAttrKey(k)
	switch val := v.(type) {
	case bool:
		return key.Bool(val)
	case int64:
		return key.Int64(val)
	case uint64:
		return key.String(fmt.Sprintf("%d", val))
	case float64:
		return key.Float64(val)
	case int8:
		return key.Int64(int64(val))
	case uint8:
		return key.Int64(int64(val))
	case int16:
		return key.Int64(int64(val))
	case uint16:
		return key.Int64(int64(val))
	case int32:
		return key.Int64(int64(val))
	case uint32:
		return key.Int64(int64(val))
	case float32:
		return key.Float64(float64(val))
	case int:
		return key.Int(val)
	case uint:
		return key.String(fmt.Sprintf("%d", val))
	case string:
		return key.String(val)
	default:
		return key.String(fmt.Sprint(v))
	}
}

func otTagToOTelAttrKey(k string) attribute.Key {
	return attribute.Key(k)
}

// Open Tracing

type spanLogger struct {
	logger     *zap.Logger
	span       opentracing.Span
	spanFields []zapcore.Field
}

func (sl spanLogger) Debug(msg string, fields ...zapcore.Field) {
	sl.logToSpan("Debug", msg, fields...)
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
	tag.Error.Set(sl.span, true)
	sl.logger.Fatal(msg, append(sl.spanFields, fields...)...)
}

// With creates a child logger, and optionally adds some context fields to that logger.
func (sl spanLogger) With(fields ...zapcore.Field) Logger {
	return spanLogger{logger: sl.logger.With(fields...), span: sl.span, spanFields: sl.spanFields}
}

func (sl spanLogger) logToSpan(level string, msg string, fields ...zapcore.Field) {
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

func (fa *fieldAdapter) AddFloat32(key string, value float32) {
	*fa = append(*fa, log.Float64(key, float64(value)))
}

func (fa *fieldAdapter) AddInt(key string, value int) {
	*fa = append(*fa, log.Int(key, value))
}

func (fa *fieldAdapter) AddInt64(key string, value int64) {
	*fa = append(*fa, log.Int64(key, value))
}

func (fa *fieldAdapter) AddInt32(key string, value int32) {
	*fa = append(*fa, log.Int64(key, int64(value)))
}

func (fa *fieldAdapter) AddInt16(key string, value int16) {
	*fa = append(*fa, log.Int64(key, int64(value)))
}

func (fa *fieldAdapter) AddInt8(key string, value int8) {
	*fa = append(*fa, log.Int64(key, int64(value)))
}

func (fa *fieldAdapter) AddUint(key string, value uint) {
	*fa = append(*fa, log.Uint64(key, uint64(value)))
}

func (fa *fieldAdapter) AddUint64(key string, value uint64) {
	*fa = append(*fa, log.Uint64(key, value))
}

func (fa *fieldAdapter) AddUint32(key string, value uint32) {
	*fa = append(*fa, log.Uint64(key, uint64(value)))
}

func (fa *fieldAdapter) AddUint16(key string, value uint16) {
	*fa = append(*fa, log.Uint64(key, uint64(value)))
}

func (fa *fieldAdapter) AddUint8(key string, value uint8) {
	*fa = append(*fa, log.Uint64(key, uint64(value)))
}

func (fa *fieldAdapter) AddUintptr(key string, value uintptr)                        {}
func (fa *fieldAdapter) AddArray(key string, marshaler zapcore.ArrayMarshaler) error { return nil }
func (fa *fieldAdapter) AddComplex128(key string, value complex128)                  {}
func (fa *fieldAdapter) AddComplex64(key string, value complex64)                    {}
func (fa *fieldAdapter) AddObject(key string, value zapcore.ObjectMarshaler) error   { return nil }
func (fa *fieldAdapter) AddReflected(key string, value interface{}) error            { return nil }
func (fa *fieldAdapter) OpenNamespace(key string)                                    {}

func (fa *fieldAdapter) AddDuration(key string, value time.Duration) {
	// TODO inefficient
	*fa = append(*fa, log.String(key, value.String()))
}

func (fa *fieldAdapter) AddTime(key string, value time.Time) {
	// TODO inefficient
	*fa = append(*fa, log.String(key, value.String()))
}

func (fa *fieldAdapter) AddBinary(key string, value []byte) {
	*fa = append(*fa, log.Object(key, value))
}

func (fa *fieldAdapter) AddByteString(key string, value []byte) {
	*fa = append(*fa, log.Object(key, value))
}

func (fa *fieldAdapter) AddString(key, value string) {
	if key != "" && value != "" {
		*fa = append(*fa, log.String(key, value))
	}
}
