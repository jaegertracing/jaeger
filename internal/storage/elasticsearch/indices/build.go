// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// BuildRotation constructs the appropriate Rotation from a resolved RotationConfig.
// The index prefix and base name are kept raw so the strategy can derive its own
// names: the dash-joined index prefix for periodic/alias rotations, or the
// dot-joined data stream name for the data_stream strategy.
func BuildRotation(indexPrefix config.IndexPrefix, baseName string, rc config.RotationConfig, remoteClusters []string, logger *zap.Logger) Rotation {
	prefix := indexPrefix.Apply(baseName)
	var r Rotation
	switch {
	case rc.DataStream.HasValue():
		ds := rc.DataStream.Get()
		r = NewDataStreamRotation(indexPrefix.DataStreamName(SpanDataStreamBaseName), ds.ReadAlias)
	case rc.ManualRollover.HasValue():
		mr := rc.ManualRollover.Get()
		writeAlias := mr.WriteAlias
		if writeAlias == "" {
			writeAlias = prefix + config.IndexSeparator + "write"
		}
		readAlias := mr.ReadAlias
		if readAlias == "" {
			readAlias = prefix + config.IndexSeparator + "read"
		}
		r = NewAliasedRotation(writeAlias, readAlias)
	case rc.AutoRollover.HasValue():
		ar := rc.AutoRollover.Get()
		writeAlias := ar.WriteAlias
		if writeAlias == "" {
			writeAlias = prefix + config.IndexSeparator + "write"
		}
		readAlias := ar.ReadAlias
		if readAlias == "" {
			readAlias = prefix + config.IndexSeparator + "read"
		}
		r = NewAliasedRotation(writeAlias, readAlias)
	case rc.Periodic.HasValue():
		p := rc.Periodic.Get()
		dateLayout := p.DateLayout
		if dateLayout == "" {
			dateLayout = "2006-01-02"
		}
		r = NewPeriodicRotation(prefix, dateLayout, rolloverFrequencyDuration(p.RolloverFrequency))
	default:
		r = NewPeriodicRotation(prefix, "2006-01-02", 24*time.Hour)
	}
	if len(remoteClusters) > 0 {
		r = NewRemoteClusterRotation(r, remoteClusters)
	}
	return NewLoggingRotation(r, logger)
}

func rolloverFrequencyDuration(frequency string) time.Duration {
	if frequency == "hour" {
		return time.Hour
	}
	return 24 * time.Hour
}
