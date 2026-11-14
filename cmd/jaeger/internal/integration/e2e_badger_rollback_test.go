// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/ports"
)

func TestBadgerStorageRollback(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageBadger)
	oldBinary := os.Getenv(oldBinaryEnvVar)
	if oldBinary == "" {
		t.Skipf("requires %s to point to a released Jaeger binary", oldBinaryEnvVar)
	}
	oldBinary, err := filepath.Abs(oldBinary)
	require.NoError(t, err)
	newBinary, err := filepath.Abs("../../jaeger-e2e")
	require.NoError(t, err)
	directory := t.TempDir()
	configFile := filepath.Join(directory, "badger.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(fmt.Sprintf(`
extensions:
  healthcheckv2:
    use_v2: true
    http:
      endpoint: localhost:13133
  jaeger_query:
    storage:
      traces: main
  jaeger_storage:
    backends:
      main:
        badger:
          directories:
            keys: %q
            values: %q
          ephemeral: false
          ttl:
            spans: 1h
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: localhost:4317
exporters:
  jaeger_storage_exporter:
    trace_storage: main
service:
  extensions: [healthcheckv2, jaeger_query, jaeger_storage]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [jaeger_storage_exporter]
`, filepath.Join(directory, "db"), filepath.Join(directory, "db"))), 0o600))

	traceID := pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	start := time.Now().Add(-time.Minute)
	makeTrace := func(firstKind ptrace.SpanKind, includeClient bool) ptrace.Traces {
		traces := ptrace.NewTraces()
		resource := traces.ResourceSpans().AppendEmpty()
		resource.Resource().Attributes().PutStr("service.name", "rollback-service")
		spans := resource.ScopeSpans().AppendEmpty().Spans()
		kinds := []ptrace.SpanKind{firstKind}
		if includeClient {
			kinds = append(kinds, ptrace.SpanKindClient)
		}
		for i, kind := range kinds {
			span := spans.AppendEmpty()
			span.SetTraceID(traceID)
			span.SetSpanID(pcommon.SpanID{byte(i + 1)})
			span.SetName("rollback-operation")
			span.SetKind(kind)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Millisecond)))
		}
		return traces
	}

	for _, phase := range []struct {
		name      string
		binary    string
		gate      string
		withKinds bool
		write     bool
		firstKind ptrace.SpanKind
	}{
		{"new_write", newBinary, "jaeger.badger.operationIndex", true, true, ptrace.SpanKindServer},
		{"new_restart", newBinary, "jaeger.badger.operationIndex", true, false, ptrace.SpanKindServer},
		{"gate_disabled", newBinary, "-jaeger.badger.operationIndex", false, false, ptrace.SpanKindServer},
		{"released_rollback_and_write", oldBinary, "", false, true, ptrace.SpanKindProducer},
		{"reupgrade", newBinary, "jaeger.badger.operationIndex", false, false, ptrace.SpanKindProducer},
	} {
		if !t.Run(phase.name, func(t *testing.T) {
			args := []string{phase.binary, "--config=" + configFile}
			if phase.gate != "" {
				args = append(args, "--feature-gates="+phase.gate)
			}
			binary := &Binary{Name: phase.name, Cmd: exec.Cmd{Path: phase.binary, Args: args}}
			binary.Start(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			reader, err := createTraceReader(zap.NewNop(), ports.QueryGRPC)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, reader.Close()) })
			if phase.write {
				writer, err := createTraceWriter(zap.NewNop(), otlpPort)
				require.NoError(t, err)
				require.NoError(t, writer.WriteTraces(ctx, makeTrace(phase.firstKind, phase.withKinds)))
				require.NoError(t, writer.Close())
			}
			want := []tracestore.Operation{{Name: "rollback-operation"}}
			if phase.withKinds {
				want = []tracestore.Operation{{Name: "rollback-operation", SpanKind: "client"}, {Name: "rollback-operation", SpanKind: "server"}}
			}
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				operations, err := reader.GetOperations(ctx, tracestore.OperationQueryParams{ServiceName: "rollback-service"})
				require.NoError(c, err)
				assert.ElementsMatch(c, want, operations)
			}, 10*time.Second, 20*time.Millisecond)
			operations, err := reader.GetOperations(ctx, tracestore.OperationQueryParams{ServiceName: "rollback-service", SpanKind: "missing"})
			require.NoError(t, err)
			if phase.withKinds {
				assert.Empty(t, operations)
				operations, err = reader.GetOperations(ctx, tracestore.OperationQueryParams{ServiceName: "rollback-service", SpanKind: "server"})
				require.NoError(t, err)
				assert.Equal(t, []tracestore.Operation{{Name: "rollback-operation", SpanKind: "server"}}, operations)
			} else {
				assert.Equal(t, want, operations)
			}
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				count := 0
				for batch, err := range reader.GetTraces(ctx, tracestore.GetTraceParams{TraceID: traceID}) {
					require.NoError(c, err)
					for _, traces := range batch {
						count += traces.SpanCount()
						for _, span := range jptrace.SpanIter(traces) {
							assert.Equal(c, traceID, span.TraceID())
							if span.SpanID() == (pcommon.SpanID{1}) {
								assert.Equal(c, phase.firstKind, span.Kind())
							}
						}
					}
				}
				assert.Equal(c, 2, count)
			}, 10*time.Second, 20*time.Millisecond)
			for _, operation := range []string{"", "rollback-operation"} {
				query := tracestore.TraceQueryParams{
					ServiceName: "rollback-service", OperationName: operation,
					StartTimeMin: start.Add(-time.Second), StartTimeMax: start.Add(time.Second),
					Attributes: pcommon.NewMap(), SearchDepth: 100,
				}
				count := 0
				for batch, err := range reader.FindTraces(ctx, query) {
					require.NoError(t, err)
					count += len(batch)
				}
				assert.Equal(t, 1, count)
			}
		}) {
			break
		}
	}
}
