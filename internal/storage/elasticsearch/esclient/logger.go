// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapLogger adapts a *zap.Logger to elastictransport.Logger, so the pool's
// per-request logging honors the Elasticsearch client's log_level and flows into
// Jaeger's logger instead of the library's stdout default.
type zapLogger struct {
	logger *zap.Logger
}

// newZapLogger builds a zapLogger for the given log-level (debug/info/error). The
// level is applied on top of the parent logger with zap.IncreaseLevel, which can
// only raise the threshold: log_level makes Elasticsearch logging quieter than the
// application logger, never more verbose. So with the app at info, log_level=error
// mutes the per-request info logs, but log_level=debug cannot surface anything the
// parent logger's own level already suppresses. This matches the pre-2.20 olivere
// client, which carried the same constraint.
func newZapLogger(logLevel string, logger *zap.Logger) *zapLogger {
	var lvl zapcore.Level
	switch logLevel {
	case "debug":
		lvl = zap.DebugLevel
	case "info":
		lvl = zap.InfoLevel
	default: // "error"
		lvl = zap.ErrorLevel
	}
	return &zapLogger{logger: logger.WithOptions(zap.IncreaseLevel(lvl))}
}

// LogRoundTrip logs one request/response exchange: method, URL, status, and
// duration. Successful round trips log at info, failures at error, so
// log_level=error surfaces only failures while log_level=info also traces every
// request. Bodies are not logged (see RequestBodyEnabled).
func (l *zapLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, _ time.Time, dur time.Duration) error {
	fields := []zap.Field{
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Duration("duration", dur),
	}
	if res != nil {
		fields = append(fields, zap.Int("status_code", res.StatusCode))
	}
	if err != nil {
		l.logger.Error("Elasticsearch request failed", append(fields, zap.Error(err))...)
		return nil
	}
	l.logger.Info("Elasticsearch request", fields...)
	return nil
}

// RequestBodyEnabled and ResponseBodyEnabled report false: this logger records the
// request line but not the bodies. Enabling capture makes the pool read and
// duplicate the full body into memory before LogRoundTrip runs; Elasticsearch
// bodies are unbounded (multi-MB bulk payloads) and carry span data, so we neither
// buffer nor log them. LogRoundTrip therefore never receives a body to log.
func (*zapLogger) RequestBodyEnabled() bool { return false }

// ResponseBodyEnabled reports false; see RequestBodyEnabled.
func (*zapLogger) ResponseBodyEnabled() bool { return false }
