// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// ResolveSpanDataStreamName fills in the resolved span data stream name on rc when
// it uses the data_stream strategy, deriving it from the raw index prefix. It is a
// no-op for every other strategy. Centralizing this keeps the data stream name in
// the RotationConfig (like the resolved aliases of the other strategies) so
// BuildRotation can read everything it needs from rc.
func ResolveSpanDataStreamName(rc *config.RotationConfig, rawIndexPrefix string) {
	if ds := rc.DataStream.Get(); ds != nil {
		ds.Name = DataStreamName(rawIndexPrefix, SpanDataStreamBaseName)
	}
}

// BuildRotation constructs the appropriate Rotation from a resolved RotationConfig.
func BuildRotation(prefix string, rc config.RotationConfig, remoteClusters []string, logger *zap.Logger) Rotation {
	var r Rotation
	switch {
	case rc.DataStream.HasValue():
		ds := rc.DataStream.Get()
		r = NewDataStreamRotation(ds.Name, ds.ReadAlias)
	case rc.ManualRollover.HasValue():
		mr := rc.ManualRollover.Get()
		writeAlias := mr.WriteAlias
		if writeAlias == "" {
			writeAlias = prefix + "write"
		}
		readAlias := mr.ReadAlias
		if readAlias == "" {
			readAlias = prefix + "read"
		}
		r = NewAliasedRotation(writeAlias, readAlias)
	case rc.AutoRollover.HasValue():
		ar := rc.AutoRollover.Get()
		writeAlias := ar.WriteAlias
		if writeAlias == "" {
			writeAlias = prefix + "write"
		}
		readAlias := ar.ReadAlias
		if readAlias == "" {
			readAlias = prefix + "read"
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
