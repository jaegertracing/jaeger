// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func TestRemoteClusterRotation(t *testing.T) {
	inner := NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour)
	r := NewRemoteClusterRotation(inner, []string{"cluster_one", "cluster_two"})

	date := time.Date(1995, time.April, 21, 4, 0, 0, 0, time.UTC)

	t.Run("WriteTarget delegates to inner", func(t *testing.T) {
		assert.Equal(t, config.SpanIndexName + config.IndexSeparator + "1995-04-21", r.WriteTarget(date))
	})

	t.Run("ReadTargets adds remote clusters", func(t *testing.T) {
		expected := []string{
			config.SpanIndexName + config.IndexSeparator + "1995-04-21",
			"cluster_one:jaeger-span-1995-04-21",
			"cluster_two:jaeger-span-1995-04-21",
		}
		assert.Equal(t, expected, r.ReadTargets(date, date))
	})

	t.Run("WriteOpType delegates to inner", func(t *testing.T) {
		assert.Equal(t, WriteOpIndex, r.WriteOpType())
	})
}

func TestRemoteClusterRotation_WithAlias(t *testing.T) {
	inner := NewAliasedRotation(config.SpanIndexName+config.IndexSeparator+"write", config.SpanIndexName+config.IndexSeparator+"read")
	r := NewRemoteClusterRotation(inner, []string{"cluster_one"})

	expected := []string{
		config.SpanIndexName + config.IndexSeparator + "read",
		"cluster_one:" + config.SpanIndexName + config.IndexSeparator + "read",
	}
	assert.Equal(t, expected, r.ReadTargets(time.Now(), time.Now()))
}

func TestRemoteClusterRotation_NoClusters(t *testing.T) {
	inner := NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour)
	r := NewRemoteClusterRotation(inner, nil)

	date := time.Date(1995, time.April, 21, 4, 0, 0, 0, time.UTC)
	assert.Equal(t, []string{config.SpanIndexName + config.IndexSeparator + "1995-04-21"}, r.ReadTargets(date, date))
}
