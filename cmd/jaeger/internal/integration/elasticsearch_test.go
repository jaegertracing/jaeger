// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func TestElasticsearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Fixtures:     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.Elasticsearch(),
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	s.RunSpanStoreTests(t)
}

func TestElasticsearchStorage_NativeTraceSummariesHTTP5xx(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)

	s := &E2EStorageIntegration{
		ConfigFile: "../../config-elasticsearch.yaml",
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.Elasticsearch(),
		},
	}
	s.e2eInitialize(t, "elasticsearch")
	purge(t)

	const serviceName = "native-summary-http-5xx"
	traceID := pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	start := time.Now().UTC()

	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, serviceName)
	span := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceID)
	span.SetSpanID(spanID)
	span.SetKind(ptrace.SpanKindServer)
	span.SetName("GET /failure")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Millisecond)))
	span.Attributes().PutInt(otelsemconv.HTTPResponseStatusCodeKey, 500)

	ctx := context.Background()
	require.NoError(t, s.TraceWriter.WriteTraces(ctx, traces))

	query := tracestore.TraceQueryParams{
		ServiceName:  serviceName,
		Attributes:   pcommon.NewMap(),
		StartTimeMin: start.Add(-time.Minute),
		StartTimeMax: start.Add(time.Minute),
		SearchDepth:  10,
	}

	var summary *tracestore.TraceSummary
	require.Eventually(t, func() bool {
		batches, err := jiter.FlattenWithErrors(s.TraceReader.FindTraceSummaries(ctx, query))
		if err != nil {
			t.Logf("FindTraceSummaries failed: %v", err)
			return false
		}
		for i := range batches {
			if batches[i].TraceID == traceID {
				candidate := batches[i]
				summary = &candidate
				break
			}
		}
		if summary == nil {
			return false
		}
		return summary.SpanCount == 1
	}, 30*time.Second, time.Second)

	require.NotNil(t, summary)
	require.Equal(t, 1, summary.ErrorSpanCount)
	require.Len(t, summary.Services, 1)
	require.Equal(t, serviceName, summary.Services[0].Name)
	require.Equal(t, 1, summary.Services[0].ErrorSpanCount)
}

func TestElasticsearchStorage_ManualRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	setupManualRolloverIndices(t, "jaeger-mr")
	runRotationSmokeTest(t, "../../config-elasticsearch-manual-rollover.yaml", "elasticsearch", func(t *testing.T) {
		initManualRolloverIndices(t, "jaeger-mr")
	})
}

func TestElasticsearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	runRotationSmokeTest(t, "../../config-elasticsearch-auto-rollover.yaml", "elasticsearch", func(t *testing.T) {
		initAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	})
}

func TestElasticsearchStorage_DataStream(t *testing.T) {
	t.Skip("data_stream rotation not yet implemented (see RFC 0004 Phase 2)")

	// No setup helper is needed because data streams auto-create on first write
	// once the composable template is in place.
	integration.SkipUnlessEnv(t, integration.StorageElasticsearch)
	runRotationSmokeTest(t, "../../config-elasticsearch-data-stream.yaml", "elasticsearch", func(*testing.T) {})
}
