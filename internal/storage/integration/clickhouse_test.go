// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type ClickHouseStorageIntegration struct {
	StorageIntegration
	factory *ch.Factory
	cfg     ch.Configuration
}

func (s *ClickHouseStorageIntegration) initialize(t *testing.T) {
	cfg := ch.DefaultConfiguration()
	cfg.Addresses = []string{"127.0.0.1:9000"}
	cfg.Database = "jaeger"
	cfg.CreateSchema = true
	// The compose file runs a single replica with an embedded Keeper, so this suite creates
	// the schema with the replicated engines, and requireReplicatedTables checks that it did.
	// The e2e suite creates the local engines through cmd/jaeger/config-clickhouse.yaml; in CI
	// the two suites run in separate containers, and against one shared server whichever
	// suite creates the tables first fixes their engine, which the check then reports.
	cfg.TableEngine = ch.TableEngine{
		Replicated: configoptional.Some(ch.ReplicatedEngine{
			KeeperPath:  "/clickhouse/tables/{shard}/{database}/{table}",
			ReplicaName: "{replica}",
		}),
	}
	cfg.Auth = ch.Authentication{
		Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
			Username: "default",
			Password: "password",
		}),
	}
	f, err := ch.NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	s.factory = f
	s.cfg = cfg
	requireReplicatedTables(t, cfg)

	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
}

// requireReplicatedTables asserts that every table the factory created carries a Replicated*
// engine. Every CREATE TABLE is IF NOT EXISTS, so without this check a schema rendered with
// the local engines would pass the suite unnoticed.
func requireReplicatedTables(t *testing.T, cfg ch.Configuration) {
	conn := openConn(t, cfg)
	rows, err := conn.Query(context.Background(),
		"SELECT name, engine FROM system.tables WHERE database = ? AND engine != 'MaterializedView' ORDER BY name",
		cfg.Database)
	require.NoError(t, err)
	defer rows.Close()
	engines := map[string]string{}
	for rows.Next() {
		var name, engine string
		require.NoError(t, rows.Scan(&name, &engine))
		engines[name] = engine
	}
	require.NoError(t, rows.Err())
	require.Equal(t, map[string]string{
		"attribute_metadata":  "ReplicatedAggregatingMergeTree",
		"dependencies":        "ReplicatedMergeTree",
		"operations":          "ReplicatedAggregatingMergeTree",
		"services":            "ReplicatedAggregatingMergeTree",
		"spans":               "ReplicatedMergeTree",
		"trace_id_timestamps": "ReplicatedAggregatingMergeTree",
	}, engines)
}

// openConn opens a connection to the database of cfg that is closed when the test ends.
func openConn(t *testing.T, cfg ch.Configuration) driver.Conn {
	basic := cfg.Auth.Basic.Get()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addresses,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: basic.Username,
			Password: string(basic.Password),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return conn
}

// testTraceIDTimestamps checks that trace_id_timestamps records the end of the
// latest-finishing span as the trace end, and that creating the schema over a database
// whose trace_id_timestamps_mv still computes max(start_time) replaces the view's query.
func (s *ClickHouseStorageIntegration) testTraceIDTimestamps(t *testing.T) {
	ctx := context.Background()
	conn := openConn(t, s.cfg)
	require.NoError(t, conn.Exec(ctx, "DROP VIEW IF EXISTS trace_id_timestamps_mv"))
	require.NoError(t, conn.Exec(ctx, `CREATE MATERIALIZED VIEW trace_id_timestamps_mv
TO trace_id_timestamps
AS
SELECT
    trace_id,
    min(start_time) AS start,
    max(start_time) AS end
FROM spans
GROUP BY trace_id`))

	f, err := ch.NewFactory(ctx, s.cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	writer, err := f.CreateTraceWriter()
	require.NoError(t, err)
	reader, err := f.CreateTraceReader()
	require.NoError(t, err)

	const service = "trace-id-timestamps-service"
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	type span struct {
		start    time.Duration
		duration time.Duration
	}
	tests := []struct {
		name    string
		traceID pcommon.TraceID
		spans   []span
		end     time.Duration
	}{
		{
			name:    "single span",
			traceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x7e, 1}),
			spans:   []span{{start: 0, duration: 5 * time.Second}},
			end:     5 * time.Second,
		},
		{
			// The span that finishes last is not the one that starts last.
			name:    "multiple spans",
			traceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x7e, 2}),
			spans: []span{
				{start: 0, duration: 10 * time.Second},
				{start: 1 * time.Second, duration: 2 * time.Second},
				{start: 3 * time.Second, duration: 1 * time.Second},
			},
			end: 10 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", service)
			ss := rs.ScopeSpans().AppendEmpty()
			for i, sp := range tt.spans {
				otelSpan := ss.Spans().AppendEmpty()
				otelSpan.SetTraceID(tt.traceID)
				otelSpan.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, tt.traceID[15], byte(i + 1)}))
				otelSpan.SetName("op")
				otelSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(sp.start)))
				otelSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(sp.start + sp.duration)))
			}
			require.NoError(t, writer.WriteTraces(ctx, td))

			var found []tracestore.FoundTraceID
			for chunk, err := range reader.FindTraceIDs(ctx, tracestore.TraceQueryParams{
				ServiceName:  service,
				Attributes:   pcommon.NewMap(),
				StartTimeMin: base.Add(-time.Minute),
				StartTimeMax: base.Add(time.Minute),
				SearchDepth:  10,
			}) {
				require.NoError(t, err)
				for _, id := range chunk.Results {
					if id.TraceID == tt.traceID {
						found = append(found, id)
					}
				}
			}
			require.Len(t, found, 1)
			assert.Equal(t, base, found[0].Start.UTC())
			assert.Equal(t, base.Add(tt.end), found[0].End.UTC())
		})
	}
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
	t.Run("TraceIDTimestamps", s.testTraceIDTimestamps)
}
