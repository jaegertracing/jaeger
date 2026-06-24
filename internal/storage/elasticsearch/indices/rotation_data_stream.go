// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import "time"

// DataStreamRotation writes spans to an Elasticsearch/OpenSearch data stream and
// reads either from the data stream name directly or, during migration from a
// legacy strategy, from an optional read alias that spans both the data stream
// and old indices.
//
// Data streams are append-only: every write uses the "create" bulk op type
// (WriteOpCreate) instead of "index". The data stream name has no date suffix —
// rollover into backing indices is handled by ES/OpenSearch via the lifecycle
// policy referenced in the data stream's index template. See RFC 0004.
type DataStreamRotation struct {
	dataStream string
	readAlias  string
}

var _ Rotation = (*DataStreamRotation)(nil)

// NewDataStreamRotation creates a DataStreamRotation for the given data stream
// name (e.g. "jaeger.spans" or "prod.jaeger.spans" when an index prefix is set).
//
// readAlias is optional: when non-empty, reads target the alias instead of the
// data stream name directly. This lets operators unify the data stream and legacy
// indices under a single alias during migration. Jaeger does not create or manage
// that alias — it only queries it (see RFC 0004 §4.1).
func NewDataStreamRotation(dataStream, readAlias string) *DataStreamRotation {
	return &DataStreamRotation{
		dataStream: dataStream,
		readAlias:  readAlias,
	}
}

func (s *DataStreamRotation) WriteTarget(time.Time) string {
	return s.dataStream
}

func (s *DataStreamRotation) ReadTargets(time.Time, time.Time) []string {
	if s.readAlias != "" {
		return []string{s.readAlias}
	}
	return []string{s.dataStream}
}

func (*DataStreamRotation) WriteOpType() WriteOpType { return WriteOpCreate }
