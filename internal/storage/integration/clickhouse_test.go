// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type ClickHouseStorageIntegration struct {
	StorageIntegration
	factory *ch.Factory
}

func (s *ClickHouseStorageIntegration) initialize(t *testing.T) {
	require.NoError(t, featuregate.GlobalRegistry().Set("storage.clickhouse", true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set("storage.clickhouse", false))
	})

	cfg := ch.Configuration{
		Addresses:    []string{"127.0.0.1:9000"},
		Database:     "jaeger",
		CreateSchema: true,
		Auth: ch.Authentication{
			Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
				Username: "default",
				Password: "password",
			}),
		},
	}
	f, err := ch.NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	s.factory = f

	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
}

func (s *ClickHouseStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func TestClickHouseStorage(t *testing.T) {
	SkipUnlessEnv(t, StorageClickHouse)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &ClickHouseStorageIntegration{
		StorageIntegration: StorageIntegration{
			Capabilities: capabilities.ClickHouse(),
		},
	}
	s.CleanUp = s.cleanUp
	s.initialize(t)
	s.RunAll(t)
	t.Run("FindTraceSummaries_Native", s.testNativeTraceSummaries)
}

func (s *ClickHouseStorageIntegration) testNativeTraceSummaries(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	sr, ok := s.TraceReader.(tracestore.SummaryReader)
	require.True(t, ok)

	base := time.Now().Add(-1 * time.Hour)
	traceID := pcommon.TraceID([16]byte{1})
	traces := ptrace.NewTraces()

	rsA := traces.ResourceSpans().AppendEmpty()
	rsA.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "svc-a")
	ssA := rsA.ScopeSpans().AppendEmpty()

	root := ssA.Spans().AppendEmpty()
	root.SetTraceID(traceID)
	root.SetSpanID(pcommon.SpanID([8]byte{1}))
	root.SetParentSpanID(pcommon.SpanID{})
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(base))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(time.Minute)))
	root.SetName("GET /root")

	errorChild := ssA.Spans().AppendEmpty()
	errorChild.SetTraceID(traceID)
	errorChild.SetSpanID(pcommon.SpanID([8]byte{3}))
	errorChild.SetParentSpanID(root.SpanID())
	errorChild.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(30 * time.Second)))
	errorChild.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(40 * time.Second)))
	errorChild.Status().SetCode(ptrace.StatusCodeError)

	rsB := traces.ResourceSpans().AppendEmpty()
	rsB.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "svc-b")
	ssB := rsB.ScopeSpans().AppendEmpty()

	orphan := ssB.Spans().AppendEmpty()
	orphan.SetTraceID(traceID)
	orphan.SetSpanID(pcommon.SpanID([8]byte{2}))
	orphan.SetParentSpanID(pcommon.SpanID([8]byte{9})) // dangling parent, not present in the trace
	orphan.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(10 * time.Second)))
	orphan.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(15 * time.Second)))

	s.writeTrace(t, traces)

	query := tracestore.TraceQueryParams{
		ServiceName:  "svc-a",
		Attributes:   pcommon.NewMap(),
		StartTimeMin: base.Add(-time.Minute),
		StartTimeMax: base.Add(2 * time.Minute),
		SearchDepth:  10,
	}

	var summaries []tracestore.TraceSummary
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		batches, err := jiter.CollectWithErrors(sr.FindTraceSummaries(context.Background(), query))
		if err != nil {
			return false
		}
		for _, batch := range batches {
			for i := range batch {
				if batch[i].TraceID == traceID {
					summaries = []tracestore.TraceSummary{batch[i]}
					return true
				}
			}
		}
		return false
	})
	require.True(t, found, "trace summary not found")

	require.Len(t, summaries, 1)
	summary := summaries[0]
	assert.Equal(t, 3, summary.SpanCount)
	assert.Equal(t, 1, summary.ErrorSpanCount)
	assert.Equal(t, 1, summary.OrphanSpanCount)
	assert.Equal(t, "svc-a", summary.RootServiceName)
	assert.Equal(t, "GET /root", summary.RootOperationName)
	assert.Equal(t, []tracestore.ServiceSummary{
		{Name: "svc-a", SpanCount: 2, ErrorSpanCount: 1},
		{Name: "svc-b", SpanCount: 1, ErrorSpanCount: 0},
	}, summary.Services)
	assert.WithinDuration(t, base, summary.MinStartTime, time.Second)
	assert.WithinDuration(t, base.Add(time.Minute), summary.MaxEndTime, time.Second)
}
