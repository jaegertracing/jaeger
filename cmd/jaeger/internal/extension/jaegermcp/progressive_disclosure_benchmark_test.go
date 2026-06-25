// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func TestMCPClientInstructionsPreferProgressiveDisclosure(t *testing.T) {
	s := connectMCPSession(t, nil)
	instructions := s.InitializeResult().Instructions

	assert.Contains(t, instructions, "Recommended workflow")
	assert.Contains(t, instructions, "use `get_trace_topology` or `get_critical_path`")
	assert.Contains(t, instructions, "Call `get_span_details` only for specific suspicious span IDs")
	assert.Contains(t, instructions, "Avoid calling `get_span_details` for every span in a large trace")
}

func TestMCPClientProgressiveDisclosureBenchmarks(t *testing.T) {
	t.Run("error heavy trace preserves compact topology before verbose error details", func(t *testing.T) {
		traceID := benchmarkTraceID(0x44)
		trace := newErrorHeavyBenchmarkTrace(traceID, 200)
		mockReader := newBenchmarkReader(trace, []string{"svc-00", "svc-01", "svc-02", "svc-03"})
		s := connectMCPSessionWithLimits(t, mockReader, 256)

		searchText := s.callTool(t, "search_traces", map[string]any{
			"service_name":   "svc-00",
			"start_time_min": "-1h",
			"with_errors":    true,
		})
		assert.Contains(t, searchText, traceID.String())

		topologyText := s.callTool(t, "get_trace_topology", map[string]any{"trace_id": traceID.String()})
		errorsText := s.callTool(t, "get_trace_errors", map[string]any{"trace_id": traceID.String()})

		var topology struct {
			TraceID string `json:"trace_id"`
			Spans   []struct {
				Path    string `json:"path"`
				Service string `json:"service"`
			} `json:"spans"`
		}
		var errorsOutput struct {
			TraceID         string `json:"trace_id"`
			TotalErrorCount int    `json:"total_error_count"`
			Spans           []struct {
				SpanID string `json:"span_id"`
			} `json:"spans"`
		}

		require.NoError(t, json.Unmarshal([]byte(topologyText), &topology))
		require.NoError(t, json.Unmarshal([]byte(errorsText), &errorsOutput))

		assert.Equal(t, traceID.String(), topology.TraceID)
		assert.Len(t, topology.Spans, 201, "root + 200 error spans should be visible in topology")
		assert.Equal(t, traceID.String(), errorsOutput.TraceID)
		assert.Equal(t, 200, errorsOutput.TotalErrorCount)
		assert.Len(t, errorsOutput.Spans, 200, "benchmark session raises the cap so the full verbose payload is measurable")

		topologyTokens := approximateTokenCount(topologyText)
		errorTokens := approximateTokenCount(errorsText)
		t.Logf("error-heavy trace: topology=%d bytes (~%d tokens), errors=%d bytes (~%d tokens)",
			len(topologyText), topologyTokens, len(errorsText), errorTokens)

		assert.Greater(t, len(errorsText), len(topologyText)*4)
		assert.Greater(t, errorTokens, topologyTokens*4)
	})

	t.Run("wide and shallow trace favors topology over full span details", func(t *testing.T) {
		traceID := benchmarkTraceID(0x55)
		trace := newWideAndShallowBenchmarkTrace(traceID, 20, 500)
		mockReader := newBenchmarkReader(trace, benchmarkServiceNames(20))
		s := connectMCPSessionWithLimits(t, mockReader, 600)

		searchText := s.callTool(t, "search_traces", map[string]any{
			"service_name":   "svc-00",
			"start_time_min": "-1h",
		})
		assert.Contains(t, searchText, traceID.String())

		topologyText := s.callTool(t, "get_trace_topology", map[string]any{"trace_id": traceID.String()})

		var topology struct {
			TraceID string `json:"trace_id"`
			Spans   []struct {
				Path    string `json:"path"`
				Service string `json:"service"`
			} `json:"spans"`
		}
		require.NoError(t, json.Unmarshal([]byte(topologyText), &topology))
		assert.Equal(t, traceID.String(), topology.TraceID)
		assert.Len(t, topology.Spans, 501, "root + 500 fan-out spans should be visible in topology")
		assert.Len(t, uniqueServices(topology.Spans), 20)

		detailsText := s.callTool(t, "get_span_details", map[string]any{
			"trace_id": traceID.String(),
			"span_ids": spanIDsFromTopology(topology.Spans),
		})

		var detailsOutput struct {
			TraceID string `json:"trace_id"`
			Spans   []struct {
				SpanID string `json:"span_id"`
			} `json:"spans"`
		}
		require.NoError(t, json.Unmarshal([]byte(detailsText), &detailsOutput))
		assert.Equal(t, traceID.String(), detailsOutput.TraceID)
		assert.Len(t, detailsOutput.Spans, 501)

		topologyTokens := approximateTokenCount(topologyText)
		detailTokens := approximateTokenCount(detailsText)
		t.Logf("wide trace: topology=%d bytes (~%d tokens), details=%d bytes (~%d tokens)",
			len(topologyText), topologyTokens, len(detailsText), detailTokens)

		assert.Greater(t, len(detailsText), len(topologyText)*3)
		assert.Greater(t, detailTokens, topologyTokens*3)
	})
}

func connectMCPSessionWithLimits(t *testing.T, mockReader *tracestoremocks.Reader, maxSpanDetails int) *mcpSession {
	t.Helper()

	svc := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	host := newMockHostWithQueryService(svc)

	server := newServer(&Config{
		HTTP: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "localhost:0",
				Transport: confignet.TransportTypeTCP,
			},
		},
		ServerName:               "jaeger",
		ServerVersion:            "1.0.0",
		MaxSpanDetailsPerRequest: maxSpanDetails,
		MaxSearchResults:         1000,
	}, componenttest.NewNopTelemetrySettings())

	require.NoError(t, server.Start(context.Background(), host))
	t.Cleanup(func() {
		assert.NoError(t, server.Shutdown(context.Background()))
	})

	addr := server.listener.Addr().String()
	waitForServer(t, addr)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	return &mcpSession{ClientSession: session, ctx: ctx}
}

func newBenchmarkReader(trace ptrace.Traces, services []string) *tracestoremocks.Reader {
	r := &tracestoremocks.Reader{}
	r.On("GetServices", mock.Anything).Return(services, nil)
	r.On("FindTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			}
		},
	)
	r.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			}
		},
	)
	return r
}

func newWideAndShallowBenchmarkTrace(traceID pcommon.TraceID, serviceCount int, childSpanCount int) ptrace.Traces {
	traces := ptrace.NewTraces()
	start := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	rootSpanID := benchmarkSpanID(1)

	rootRS := traces.ResourceSpans().AppendEmpty()
	rootRS.Resource().Attributes().PutStr("service.name", "svc-00")
	rootSS := rootRS.ScopeSpans().AppendEmpty()
	root := rootSS.Spans().AppendEmpty()
	root.SetTraceID(traceID)
	root.SetSpanID(rootSpanID)
	root.SetName("HTTP GET /checkout")
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(6 * time.Second)))
	root.Status().SetCode(ptrace.StatusCodeOk)

	serviceScopes := make(map[string]ptrace.ScopeSpans, serviceCount)
	for i := 0; i < childSpanCount; i++ {
		serviceName := fmt.Sprintf("svc-%02d", i%serviceCount)
		scope := ensureServiceScope(traces, serviceScopes, serviceName)

		span := scope.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(benchmarkSpanID(uint64(i + 2)))
		span.SetParentSpanID(rootSpanID)
		span.SetName(fmt.Sprintf("fanout-%03d", i))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(i%40) * time.Millisecond)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(40*time.Millisecond + time.Duration(i%40)*time.Millisecond)))
		span.Status().SetCode(ptrace.StatusCodeOk)
		span.Attributes().PutStr("peer.service", serviceName)
		span.Attributes().PutStr("http.route", fmt.Sprintf("/services/%02d", i%serviceCount))
		span.Attributes().PutStr("benchmark.payload", strings.Repeat("x", 160))
		event := span.Events().AppendEmpty()
		event.SetName("fanout.dispatch")
		event.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(i%40) * time.Millisecond)))
		event.Attributes().PutStr("sql", strings.Repeat("SELECT 1 ", 24))
	}

	return traces
}

func newErrorHeavyBenchmarkTrace(traceID pcommon.TraceID, errorSpanCount int) ptrace.Traces {
	traces := ptrace.NewTraces()
	start := time.Date(2026, time.January, 1, 13, 0, 0, 0, time.UTC)
	rootSpanID := benchmarkSpanID(1)

	rootRS := traces.ResourceSpans().AppendEmpty()
	rootRS.Resource().Attributes().PutStr("service.name", "svc-00")
	rootSS := rootRS.ScopeSpans().AppendEmpty()
	root := rootSS.Spans().AppendEmpty()
	root.SetTraceID(traceID)
	root.SetSpanID(rootSpanID)
	root.SetName("POST /payments")
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(4 * time.Second)))
	root.Status().SetCode(ptrace.StatusCodeOk)

	serviceScopes := make(map[string]ptrace.ScopeSpans, 4)
	stacktrace := strings.Repeat("panic: timeout waiting on downstream dependency\n", 16)
	for i := 0; i < errorSpanCount; i++ {
		serviceName := fmt.Sprintf("svc-%02d", i%4)
		scope := ensureServiceScope(traces, serviceScopes, serviceName)

		span := scope.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(benchmarkSpanID(uint64(i + 2)))
		span.SetParentSpanID(rootSpanID)
		span.SetName(fmt.Sprintf("retry-%03d", i))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(i%50) * time.Millisecond)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(20*time.Millisecond + time.Duration(i%50)*time.Millisecond)))
		span.Status().SetCode(ptrace.StatusCodeError)
		span.Status().SetMessage("connection refused")
		span.Attributes().PutStr("error.type", "io.timeout")
		span.Attributes().PutStr("exception.stacktrace", stacktrace)
		span.Attributes().PutStr("http.url", fmt.Sprintf("https://svc-%02d.internal/payments", i%4))
		event := span.Events().AppendEmpty()
		event.SetName("exception")
		event.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(i%50) * time.Millisecond)))
		event.Attributes().PutStr("exception.type", "timeout")
		event.Attributes().PutStr("exception.message", strings.Repeat("deadline exceeded ", 20))
	}

	return traces
}

func ensureServiceScope(
	traces ptrace.Traces,
	scopes map[string]ptrace.ScopeSpans,
	serviceName string,
) ptrace.ScopeSpans {
	if scope, ok := scopes[serviceName]; ok {
		return scope
	}

	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", serviceName)
	scope := rs.ScopeSpans().AppendEmpty()
	scopes[serviceName] = scope
	return scope
}

func benchmarkTraceID(seed byte) pcommon.TraceID {
	var traceID pcommon.TraceID
	for i := range traceID {
		traceID[i] = seed
	}
	traceID[len(traceID)-1] = seed + 1
	return traceID
}

func benchmarkSpanID(v uint64) pcommon.SpanID {
	var spanID pcommon.SpanID
	binary.BigEndian.PutUint64(spanID[:], v)
	return spanID
}

func approximateTokenCount(text string) int {
	return (len(text) + 3) / 4
}

func benchmarkServiceNames(serviceCount int) []string {
	names := make([]string, 0, serviceCount)
	for i := 0; i < serviceCount; i++ {
		names = append(names, fmt.Sprintf("svc-%02d", i))
	}
	return names
}

func uniqueServices(spans []struct {
	Path    string `json:"path"`
	Service string `json:"service"`
},
) map[string]struct{} {
	services := make(map[string]struct{}, len(spans))
	for _, span := range spans {
		services[span.Service] = struct{}{}
	}
	return services
}

func spanIDsFromTopology(spans []struct {
	Path    string `json:"path"`
	Service string `json:"service"`
},
) []string {
	spanIDs := make([]string, 0, len(spans))
	for _, span := range spans {
		idx := strings.LastIndex(span.Path, "/")
		if idx >= 0 {
			spanIDs = append(spanIDs, span.Path[idx+1:])
			continue
		}
		spanIDs = append(spanIDs, span.Path)
	}
	return spanIDs
}
