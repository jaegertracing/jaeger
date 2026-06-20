// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	"go.uber.org/zap"
)

// LoggingRotation wraps a Rotation with debug logging of ReadTargets output.
type LoggingRotation struct {
	inner  Rotation
	logger *zap.Logger
}

var _ Rotation = (*LoggingRotation)(nil)

// NewLoggingRotation returns a LoggingRotation if the logger has debug enabled,
// otherwise returns the inner rotation unwrapped.
func NewLoggingRotation(inner Rotation, logger *zap.Logger) Rotation {
	if !logger.Core().Enabled(zap.DebugLevel) {
		return inner
	}
	return &LoggingRotation{inner: inner, logger: logger}
}

func (l *LoggingRotation) WriteTarget(spanTime time.Time) string {
	return l.inner.WriteTarget(spanTime)
}

func (l *LoggingRotation) ReadTargets(startTime, endTime time.Time) []string {
	targets := l.inner.ReadTargets(startTime, endTime)
	l.logger.Debug("Reading from ES indices", zap.Strings("index", targets))
	return targets
}

func (l *LoggingRotation) WriteOpType() WriteOpType { return l.inner.WriteOpType() }
