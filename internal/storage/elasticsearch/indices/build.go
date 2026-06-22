// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// BuildRotation constructs the appropriate Rotation from a resolved RotationConfig.
// dataStreamName is the resolved data stream name (e.g. "prod.jaeger.spans") used
// only by the data_stream variant; it is empty for index types that do not support
// data streams.
func BuildRotation(prefix, dataStreamName string, rc config.RotationConfig, remoteClusters []string, logger *zap.Logger) Rotation {
	var r Rotation
	switch {
	case rc.DataStream.HasValue():
		ds := rc.DataStream.Get()
		r = NewDataStreamRotation(dataStreamName, ds.ReadAlias)
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
