// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// clickhouseMultiNodeEnv is set by scripts/e2e/clickhouse.sh when the backend is the two-server
// cluster. The single-node suite must not then replace the hand-built Distributed schema.
const clickhouseMultiNodeEnv = "CLICKHOUSE_MULTI_NODE"

type ClickHouseStorageIntegration struct {
	StorageIntegration
	factory *ch.Factory
	config  ch.Configuration
}

func (s *ClickHouseStorageIntegration) initialize(t *testing.T) {
	s.initializeWithSchema(t, true)
}

func (s *ClickHouseStorageIntegration) initializeWithSchema(t *testing.T, createSchema bool) {
	cfg := ch.DefaultConfiguration()
	cfg.Addresses = []string{"127.0.0.1:9000"}
	cfg.Database = "jaeger"
	cfg.CreateSchema = createSchema
	if createSchema {
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
	s.config = cfg
	if createSchema {
		requireReplicatedTables(t, cfg)
	} else {
		requireDistributedTables(t, cfg)
	}

	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
}

func openClickHouse(t *testing.T, cfg ch.Configuration) driver.Conn {
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
	return conn
}

func tableEngines(t *testing.T, cfg ch.Configuration) map[string]string {
	conn := openClickHouse(t, cfg)
	defer conn.Close()
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
	return engines
}

// requireReplicatedTables asserts that every table the factory created carries a Replicated*
// engine. Every CREATE TABLE is IF NOT EXISTS, so without this check a schema rendered with
// the local engines would pass the suite unnoticed.
func requireReplicatedTables(t *testing.T, cfg ch.Configuration) {
	require.Equal(t, map[string]string{
		"attribute_metadata":  "ReplicatedAggregatingMergeTree",
		"dependencies":        "ReplicatedMergeTree",
		"operations":          "ReplicatedAggregatingMergeTree",
		"services":            "ReplicatedAggregatingMergeTree",
		"spans":               "ReplicatedMergeTree",
		"trace_id_timestamps": "ReplicatedAggregatingMergeTree",
	}, tableEngines(t, cfg))
}

// requireDistributedTables asserts that the harness applied schema-multi-node.sql: Jaeger's table
// names are Distributed wrappers, and every wrapper has a Replicated* local table behind it.
func requireDistributedTables(t *testing.T, cfg ch.Configuration) {
	require.Equal(t, map[string]string{
		"attribute_metadata":        "Distributed",
		"attribute_metadata_local":  "ReplicatedAggregatingMergeTree",
		"dependencies":              "Distributed",
		"dependencies_local":        "ReplicatedMergeTree",
		"operations":                "Distributed",
		"operations_local":          "ReplicatedAggregatingMergeTree",
		"services":                  "Distributed",
		"services_local":            "ReplicatedAggregatingMergeTree",
		"spans":                     "Distributed",
		"spans_local":               "ReplicatedMergeTree",
		"trace_id_timestamps":       "Distributed",
		"trace_id_timestamps_local": "ReplicatedAggregatingMergeTree",
	}, tableEngines(t, cfg))
}

func (s *ClickHouseStorageIntegration) cleanUp(t *testing.T) {
	if os.Getenv(clickhouseMultiNodeEnv) == "true" {
		conn := openClickHouse(t, s.config)
		defer conn.Close()
		// Table identifiers are compile-time constants from s.config and this slice;
		// ClickHouse DDL does not admit parameterized identifiers.
		tables := []string{
			"spans_local",
			"services_local",
			"operations_local",
			"trace_id_timestamps_local",
			"attribute_metadata_local",
			"dependencies_local",
		}
		for _, tbl := range tables {
			require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("TRUNCATE TABLE IF EXISTS %s.%s ON CLUSTER jaeger_cluster", s.config.Database, tbl)))
		}
		return
	}
	require.NoError(t, s.factory.Purge(context.Background()))
}

func TestClickHouseStorage(t *testing.T) {
	SkipUnlessEnv(t, StorageClickHouse)
	if os.Getenv(clickhouseMultiNodeEnv) == "true" {
		t.Skip("The multi-node suite owns the hand-built Distributed schema")
	}
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
}

// TestClickHouseStorageMultiNode runs the unchanged corpus against Distributed wrappers and then
// asserts the cost of the trace search. A result-only suite cannot distinguish a pushed-down
// query from one that scans every timestamp row and happens to return the same traces.
func TestClickHouseStorageMultiNode(t *testing.T) {
	SkipUnlessEnv(t, StorageClickHouse)
	if os.Getenv(clickhouseMultiNodeEnv) != "true" {
		t.Skipf("This test requires environment variable %s=true", clickhouseMultiNodeEnv)
	}
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &ClickHouseStorageIntegration{
		StorageIntegration: StorageIntegration{
			Capabilities: capabilities.ClickHouse(),
		},
	}
	s.CleanUp = s.cleanUp
	s.initializeWithSchema(t, false)
	s.Corpus = BuildCorpus(t, s.Fixtures, s.Capabilities)
	s.cleanUp(t)
	defer s.cleanUp(t)
	s.WriteCorpus(t)
	s.AssertCorpus(t)
	t.Run("QueryCosts", s.assertDistributedQueryCosts)
}

// assertDistributedQueryCosts runs one trace search with a query ID and reads its statements back
// from system.query_log. The bounds lookup may read only a small multiple of the number of traces
// returned; the old Distributed shape read one timestamp row per trace in the corpus instead.
func (s *ClickHouseStorageIntegration) assertDistributedQueryCosts(t *testing.T) {
	conn := openClickHouse(t, s.config)
	defer conn.Close()
	require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("OPTIMIZE TABLE %s.trace_id_timestamps_local ON CLUSTER jaeger_cluster FINAL", s.config.Database)))
	require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("OPTIMIZE TABLE %s.spans_local ON CLUSTER jaeger_cluster FINAL", s.config.Database)))

	spans := s.Corpus.Bounded.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	start := spans.At(0).StartTimestamp().AsTime()
	queryID := fmt.Sprintf("jaeger-distributed-test-%d", time.Now().UnixNano())
	ctx := clickhouse.Context(context.Background(), clickhouse.WithQueryID(queryID))
	traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(s.TraceReader.FindTraces(ctx, tracestore.TraceQueryParams{
		ServiceName:  "bounded-trace-service",
		StartTimeMin: start.Add(-time.Minute),
		StartTimeMax: start.Add(time.Minute),
		SearchDepth:  10,
		Attributes:   pcommon.NewMap(),
	})))
	require.NoError(t, err)
	require.NotEmpty(t, traces)

	require.NoError(t, conn.Exec(context.Background(), "SYSTEM FLUSH LOGS"))
	logs := readQueryLog(t, conn, queryID)
	var boundsQuery, spanQuery string
	var boundsRows, spanRows uint64
	for _, entry := range logs {
		switch {
		case strings.Contains(entry.query, "FROM trace_id_timestamps"):
			boundsQuery = entry.query
			boundsRows += entry.readRows
		case strings.Contains(entry.query, "SELECT DISTINCT"):
			// The candidate search is asserted by the corpus itself.
		case strings.Contains(entry.query, "FROM spans") || strings.Contains(entry.query, "spans s"):
			spanQuery = entry.query
			spanRows += entry.readRows
		default:
		}
	}

	t.Run("TimestampLookupReadsOnlyReturnedTraces", func(t *testing.T) {
		require.NotEmpty(t, boundsQuery, "no trace_id_timestamps lookup was logged")
		// Four leaves room for unmerged parts and version-specific accounting without allowing
		// the corpus-wide scan that the broken Distributed plan produced.
		require.LessOrEqual(t, boundsRows, uint64(4*len(traces)),
			"timestamp lookup read %d rows for %d returned traces", boundsRows, len(traces))
	})

	t.Run("SpanFetchReadsOnlyReturnedPartitions", func(t *testing.T) {
		require.NotEmpty(t, spanQuery, "no span fetch was logged")
		partitionRows := countSpanPartitionRows(t, conn, traces)
		require.LessOrEqual(t, spanRows, partitionRows,
			"span fetch read %d rows, partitions of returned traces hold %d", spanRows, partitionRows)
	})

	// EXPLAIN needs no timing or row-count stability: the read must carry the trace_id
	// predicate instead of asking the shard for the whole lookup table.
	cases := []struct {
		name  string
		query string
	}{
		{name: "TimestampLookup", query: boundsQuery},
		{name: "SpanFetch", query: spanQuery},
	}
	for _, tc := range cases {
		t.Run(tc.name+"PushesTraceIDFilter", func(t *testing.T) {
			require.NotEmpty(t, tc.query)
			plan := explainQuery(t, conn, tc.query)
			require.Contains(t, plan, "trace_id")
			require.True(t, strings.Contains(plan, "ReadFromRemote") || strings.Contains(plan, "ReadFromMergeTree"),
				"query plan %s does not read through the Distributed wrapper", plan)
		})
	}
}

type queryLogEntry struct {
	query    string
	readRows uint64
}

func readQueryLog(t *testing.T, conn driver.Conn, queryID string) []queryLogEntry {
	rows, err := conn.Query(context.Background(), `
		SELECT query, read_rows
		FROM system.query_log
		WHERE initial_query_id = ? AND type = 'QueryFinish' AND is_initial_query = 1`,
		queryID)
	require.NoError(t, err)
	defer rows.Close()
	var entries []queryLogEntry
	for rows.Next() {
		var entry queryLogEntry
		require.NoError(t, rows.Scan(&entry.query, &entry.readRows))
		entries = append(entries, entry)
	}
	require.NoError(t, rows.Err())
	return entries
}

// countSpanPartitionRows returns how many span rows live in the partitions the returned traces
// touch. A trace_id IN (...) without the time bounds reads every partition instead.
func countSpanPartitionRows(t *testing.T, conn driver.Conn, traces []ptrace.Traces) uint64 {
	days := map[string]struct{}{}
	for _, trace := range traces {
		for _, span := range jptrace.SpanIter(trace) {
			days[span.StartTimestamp().AsTime().UTC().Format("2006-01-02")] = struct{}{}
		}
	}
	require.NotEmpty(t, days)
	list := make([]string, 0, len(days))
	for day := range days {
		list = append(list, day)
	}
	row := conn.QueryRow(context.Background(),
		"SELECT count() FROM spans WHERE toDate(start_time) IN (?)", list)
	var count uint64
	require.NoError(t, row.Scan(&count))
	return count
}

// explainQuery runs EXPLAIN on query. The query string is not user input: it is read back
// from ClickHouse's own system.query_log for the test's queryID.
func explainQuery(t *testing.T, conn driver.Conn, query string) string {
	rows, err := conn.Query(context.Background(), "EXPLAIN actions=1, description=1 "+query)
	require.NoError(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	return plan.String()
}
