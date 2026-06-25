// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TestBadgerBackwardCompatibility verifies that a trace written by a previous
// Jaeger binary remains readable by the current branch binary, simulating a
// rolling upgrade with a shared on-disk Badger store. Set BACKWARD_COMPATIBILITY=true
// and optionally JAEGER_BACKWARD_COMPAT_BINARY to a local path; otherwise use
// make jaeger-v2-backward-compatibility-test which provisions the @main binary.
func TestBadgerBackwardCompatibility(t *testing.T) {
	if os.Getenv("BACKWARD_COMPATIBILITY") != "true" {
		t.Skip("set BACKWARD_COMPATIBILITY=true to run backward compatibility tests")
	}
	integration.SkipUnlessEnv(t, integration.StorageBadger)

	trace, traceID := backwardCompatFixture()

	writePhase := &E2EStorageIntegration{
		BinaryName:          "jaeger-writer",
		BinaryPath:          os.Getenv("JAEGER_BACKWARD_COMPAT_BINARY"),
		ConfigFile:          "../../config-badger.yaml",
		SkipMetricsScraping: true,
	}
	writePhase.e2eInitialize(t, "badger")
	purge(t)
	require.NoError(t, writePhase.TraceWriter.WriteTraces(context.Background(), trace))
	readTrace(t, writePhase.TraceReader, traceID)
	writePhase.binary.Stop(t)

	readPhase := &E2EStorageIntegration{
		ConfigFile:          "../../config-badger.yaml",
		SkipMetricsScraping: true,
	}
	readPhase.e2eInitialize(t, "badger")
	integration.CompareTraces(t, trace, readTrace(t, readPhase.TraceReader, traceID))
	purge(t)
}

// backwardCompatFixture returns a single-span trace with a deterministic trace ID.
func backwardCompatFixture() (ptrace.Traces, pcommon.TraceID) {
	traceID := pcommon.TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x42}
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "backward-compat-service")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceID)
	span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	span.SetName("backward-compat-operation")
	now := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Second)))
	return traces, traceID
}

// readTrace polls until the trace with the given ID is retrievable and returns it.
func readTrace(t *testing.T, reader tracestore.Reader, traceID pcommon.TraceID) ptrace.Traces {
	var actual ptrace.Traces
	require.Eventually(t, func() bool {
		seq := reader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: traceID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(seq))
		if err != nil {
			t.Log(err)
			return false
		}
		if len(traces) != 1 {
			return false
		}
		actual = traces[0]
		return true
	}, 30*time.Second, 100*time.Millisecond, "trace %s was not readable", traceID)
	return actual
}
