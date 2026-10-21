// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func TestAliasedRotation_WriteTarget(t *testing.T) {
	r := NewAliasedRotation("jaeger-span-write", "jaeger-span-read")
	date := time.Date(2024, time.June, 18, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-write", r.WriteTarget(date))
}

func TestAliasedRotation_ReadTargets(t *testing.T) {
	r := NewAliasedRotation("jaeger-span-write", "jaeger-span-read")
	start := time.Date(2024, time.June, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.June, 18, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, []string{"jaeger-span-read"}, r.ReadTargets(start, end))
}

func TestAliasedRotation_WriteOpType(t *testing.T) {
	r := NewAliasedRotation("jaeger-span-write", "jaeger-span-read")
	assert.Equal(t, es.WriteOpIndex, r.WriteOpType())
}

func TestAliasedRotation_RequiresDocumentTimestamp(t *testing.T) {
	r := NewAliasedRotation("jaeger-span-write", "jaeger-span-read")
	assert.False(t, r.RequiresDocumentTimestamp())
}
