// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func TestDataStreamRotation_WriteTarget(t *testing.T) {
	r := NewDataStreamRotation("jaeger.spans", "")
	date := time.Date(2024, time.June, 18, 10, 0, 0, 0, time.UTC)
	// The write target is the data stream name itself, with no date suffix.
	assert.Equal(t, "jaeger.spans", r.WriteTarget(date))
}

func TestDataStreamRotation_ReadTargets_DataStreamName(t *testing.T) {
	r := NewDataStreamRotation("jaeger.spans", "")
	start := time.Date(2024, time.June, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.June, 18, 0, 0, 0, 0, time.UTC)
	// With no read alias configured, reads go to the data stream name directly.
	assert.Equal(t, []string{"jaeger.spans"}, r.ReadTargets(start, end))
}

func TestDataStreamRotation_ReadTargets_ReadAliasOverride(t *testing.T) {
	r := NewDataStreamRotation("jaeger.spans", "jaeger-legacy-read-alias")
	start := time.Date(2024, time.June, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.June, 18, 0, 0, 0, 0, time.UTC)
	// When a read alias is configured (migration), reads target the alias instead.
	assert.Equal(t, []string{"jaeger-legacy-read-alias"}, r.ReadTargets(start, end))
}

func TestDataStreamRotation_WriteOpType(t *testing.T) {
	r := NewDataStreamRotation("jaeger.spans", "")
	// Data streams are append-only and require the "create" bulk op type.
	assert.Equal(t, es.WriteOpCreate, r.WriteOpType())
}

func TestDataStreamRotation_RequiresDocumentTimestamp(t *testing.T) {
	r := NewDataStreamRotation("jaeger.spans", "")
	// Data streams have no date-suffixed index, so documents must carry a timestamp.
	assert.True(t, r.RequiresDocumentTimestamp())
}
