// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"os"
	"regexp"
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
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// clickhouseMultiNodeEnv is set by scripts/e2e/clickhouse.sh when the backend is the two-server
// cluster. The single-node suite must not run against the Distributed database in that mode.
const clickhouseMultiNodeEnv = "CLICKHOUSE_MULTI_NODE"

var traceIDPredicatePattern = regexp.MustCompile(
	`\b(?:in|equals)\((?:__table\d+\.)?trace_id\b|\b(?:__table\d+\.)?trace_id\s+(?:IN\s*\(|=)`,
)

type ClickHouseStorageIntegration struct {
	StorageIntegration
	factory *ch.Factory
	config  ch.Configuration
}

func (s *ClickHouseStorageIntegration) initialize(
	t *testing.T,
	database string,
	attributeLayout string,
	hoistedAttributes []ch.HoistedAttribute,
	clusterName string,
) {
	cfg := ch.DefaultConfiguration()
	cfg.Addresses = []string{"127.0.0.1:9000"}
	cfg.Database = database
	cfg.AttributeLayout = attributeLayout
	cfg.HoistedAttributes = hoistedAttributes
	cfg.CreateSchema = true
	if clusterName != "" {
		// Multi-node runs against a Replicated database created by scripts/e2e/clickhouse.sh;
		// ClickHouse rejects explicit keeper_path/replica_name arguments there (RFC 0022 §6.7).
		cfg.TableEngine = ch.TableEngine{
			Replicated: configoptional.Some(ch.ReplicatedEngine{}),
			Cluster:    configoptional.Some(ch.ClusterConfig{Name: clusterName}),
		}
	} else {
		// The single-node compose file runs an Atomic database with an embedded Keeper, so
		// this suite creates the schema with explicit replicated paths, and requireReplicatedTables
		// checks that it did. The e2e suite creates local engines through cmd/jaeger/config-clickhouse.yaml.
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
	if clusterName != "" {
		requireDistributedTables(t, cfg)
	} else {
		requireReplicatedTables(t, cfg)
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
	var username, password string
	if basic != nil {
		username = basic.Username
		password = string(basic.Password)
	}
	return openClickHouseWithAuth(t, cfg.Addresses, cfg.Database, username, password)
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

// requireDistributedTables asserts that Jaeger created the Distributed layout (RFC 0022 §6.7):
// every public table name is a Distributed wrapper over a Replicated* local table.
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
		basic := s.config.Auth.Basic.Get()
		var username, password string
		if basic != nil {
			username = basic.Username
			password = string(basic.Password)
		}
		TruncateClickHouseLocalTables(t, s.config.Addresses, s.config.Database, username, password)
		return
	}
	require.NoError(t, s.factory.Purge(context.Background()))
}

// runClickHouseStorageSuite runs the storage suite against the tables of database, which the
// backend creates in attributeLayout and with hoistedAttributes.
func runClickHouseStorageSuite(t *testing.T, database string, attributeLayout string, hoistedAttributes []ch.HoistedAttribute) {
	SkipUnlessEnv(t, StorageClickHouse)
	if os.Getenv(clickhouseMultiNodeEnv) == "true" {
		t.Skip("The multi-node suite owns the Distributed schema in jaeger")
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
	s.initialize(t, database, attributeLayout, hoistedAttributes, "")
	s.RunAll(t)
}

func TestClickHouseStorage(t *testing.T) {
	runClickHouseStorageSuite(t, "jaeger", "nested", nil)
}

// runClickHouseStorageSuiteWithLayout runs the storage suite against a database that it
// creates anew, whose tables are in attributeLayout and have the hoisted columns. Each such
// run needs a database of its own, because the backend neither adds a column to an existing
// table nor converts its layout.
func runClickHouseStorageSuiteWithLayout(t *testing.T, database string, attributeLayout string) {
	SkipUnlessEnv(t, StorageClickHouse)
	if os.Getenv(clickhouseMultiNodeEnv) == "true" {
		t.Skip("The multi-node suite owns the Distributed schema in jaeger")
	}
	RecreateClickHouseDatabase(t, database)
	runClickHouseStorageSuite(t, database, attributeLayout, ClickHouseHoistedAttributes)
}

// TestClickHouseStorage_HoistedAttributes runs the same suite against tables with hoisted
// columns, so every search must return what it returns without them.
func TestClickHouseStorage_HoistedAttributes(t *testing.T) {
	runClickHouseStorageSuiteWithLayout(t, "jaeger_hoisted", "nested")
}

// TestClickHouseStorage_MapLayout runs the same suite against tables in the map layout, so
// every search must return what it returns from the nested layout. The tables also have the
// hoisted columns, which in this layout are computed from a map.
func TestClickHouseStorage_MapLayout(t *testing.T) {
	runClickHouseStorageSuiteWithLayout(t, "jaeger_map", "map")
}

// TestClickHouseStorageMultiNode runs the unchanged corpus against Jaeger's Distributed schema
// and then asserts the cost and query plan of trace search. A result-only suite cannot distinguish
// a pushed-down query from one that scans every timestamp row or partition.
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
	s.initialize(t, "jaeger", "nested", nil, "jaeger_cluster")
	s.Corpus = BuildCorpus(t, s.Fixtures, s.Capabilities)
	s.cleanUp(t)
	defer s.cleanUp(t)
	s.WriteCorpus(t)
	s.AssertCorpus(t)
	t.Run("QueryCosts", s.assertDistributedQueryCosts)
}

// assertDistributedQueryCosts writes an extra historical trace for bounded-trace-service so the
// service spans distinct day partitions, runs one trace search forcing remote replica execution
// (prefer_localhost_replica = 0) with a tracked query ID, and checks query_log, remote execution,
// and distributed EXPLAIN plans (filter pushdown inside ReadFromRemote + index pruning), as well
// as local-replica execution (prefer_localhost_replica = 1).
func (s *ClickHouseStorageIntegration) assertDistributedQueryCosts(t *testing.T) {
	conn := openClickHouse(t, s.config)
	defer conn.Close()

	basic := s.config.Auth.Basic.Get()
	remoteConn := openClickHouseWithAuth(t, []string{"127.0.0.1:9001"}, s.config.Database, basic.Username, string(basic.Password))
	defer remoteConn.Close()

	spans := s.Corpus.Bounded.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	start := spans.At(0).StartTimestamp().AsTime()

	// Write a trace for bounded-trace-service 4 days back so SpanFetchReadsOnlyReturnedPartitions
	// always faces at least two day partitions regardless of UTC time-of-day or corpus shifts.
	oldTrace := buildBoundedTraceInPast(start.Add(-4 * 24 * time.Hour))
	require.NoError(t, s.TraceWriter.WriteTraces(context.Background(), oldTrace))
	require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("SYSTEM FLUSH DISTRIBUTED %s.spans", s.config.Database)))
	require.Eventually(t, func() bool {
		var count uint64
		row := conn.QueryRow(context.Background(), "SELECT count() FROM spans WHERE trace_id = ?", pcommon.TraceID{0xB2}.String())
		return row.Scan(&count) == nil && count == 1
	}, 10*time.Second, 100*time.Millisecond)

	// Identifiers are compile-time constants; ON CLUSTER merges parts across all replicas.
	require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("OPTIMIZE TABLE %s.trace_id_timestamps_local ON CLUSTER jaeger_cluster FINAL", s.config.Database)))
	require.NoError(t, conn.Exec(context.Background(), fmt.Sprintf("OPTIMIZE TABLE %s.spans_local ON CLUSTER jaeger_cluster FINAL", s.config.Database)))

	queryParams := tracestore.TraceQueryParams{
		ServiceName:  "bounded-trace-service",
		StartTimeMin: start.Add(-time.Minute),
		StartTimeMax: start.Add(time.Minute),
		SearchDepth:  10,
		Attributes:   pcommon.NewMap(),
	}

	remoteRes := s.runMeasuredFindTraces(t, conn, remoteConn, queryParams, 0)
	require.True(t, hasQueryTargetingTable(remoteRes.remoteLogs, "trace_id_timestamps_local"),
		"remote replica (clickhouse-02) did not execute trace_id_timestamps_local lookup when prefer_localhost_replica=0; logs: %+v", remoteRes.remoteLogs)
	require.True(t, hasQueryTargetingTable(remoteRes.remoteLogs, "spans_local"),
		"remote replica (clickhouse-02) did not execute spans_local fetch when prefer_localhost_replica=0; logs: %+v", remoteRes.remoteLogs)

	t.Run("TimestampLookupPushesTraceIDFilter", func(t *testing.T) {
		require.NotEmpty(t, remoteRes.boundsQuery, "no trace_id_timestamps lookup was logged")
		plan := explainQuery(t, conn, remoteRes.boundsQuery, true, 0)
		subtrees := assertRemotePlanPushesTraceIDFilter(t, plan)
		for _, subtree := range subtrees {
			assertIndexCondition(t, subtree, "trace_id", "PrimaryKey")
		}
	})

	t.Run("SpanFetchReadsOnlyReturnedPartitions", func(t *testing.T) {
		require.NotEmpty(t, remoteRes.spanQuery, "no span fetch was logged")
		partitionRows := countSpanPartitionRows(t, conn, remoteRes.traces)
		totalRows := countAllSpanRows(t, conn)
		require.Less(t, partitionRows, totalRows,
			"returned traces cover %d rows which equals total table %d; partition check cannot bite", partitionRows, totalRows)
		require.LessOrEqual(t, remoteRes.spanRows, partitionRows,
			"span fetch read %d rows, partitions of returned traces hold %d", remoteRes.spanRows, partitionRows)
		plan := explainQuery(t, conn, remoteRes.spanQuery, true, 0)
		subtrees := extractPlanSubtrees(plan, "ReadFromRemote")
		require.NotEmpty(t, subtrees, "distributed plan did not include ReadFromRemote:\n%s", plan)
		for _, subtree := range subtrees {
			assertIndexCondition(t, subtree, "toDate(start_time)", "Partition")
			assertIndexCondition(t, subtree, "start_time", "MinMax", "Min-Max")
		}
	})

	t.Run("SpanFetchPushesTraceIDFilter", func(t *testing.T) {
		require.NotEmpty(t, remoteRes.spanQuery, "no span fetch was logged")
		plan := explainQuery(t, conn, remoteRes.spanQuery, true, 0)
		subtrees := assertRemotePlanPushesTraceIDFilter(t, plan)
		for _, subtree := range subtrees {
			require.Contains(t, subtree, "idx_trace_id",
				"remote span fetch subtree did not include bloom_filter skip index idx_trace_id:\n%s", subtree)
		}
	})

	t.Run("LocalReplicaExecution", func(t *testing.T) {
		localRes := s.runMeasuredFindTraces(t, conn, remoteConn, queryParams, 1)
		require.Empty(t, localRes.remoteLogs,
			"expected prefer_localhost_replica=1 to stay on local replica, but remote replica logged: %+v", localRes.remoteLogs)
		require.NotEmpty(t, localRes.boundsQuery, "no local trace_id_timestamps lookup was logged")
		require.NotEmpty(t, localRes.spanQuery, "no local span fetch was logged")

		boundsPlan := explainQuery(t, conn, localRes.boundsQuery, true, 1)
		assertLocalPlanPushesTraceIDFilter(t, boundsPlan)
		assertIndexCondition(t, boundsPlan, "trace_id", "PrimaryKey")

		spanPlan := explainQuery(t, conn, localRes.spanQuery, true, 1)
		assertLocalPlanPushesTraceIDFilter(t, spanPlan)
		assertIndexCondition(t, spanPlan, "toDate(start_time)", "Partition")
		assertIndexCondition(t, spanPlan, "start_time", "MinMax", "Min-Max")
		require.Contains(t, spanPlan, "idx_trace_id",
			"local span fetch plan did not include bloom_filter skip index idx_trace_id:\n%s", spanPlan)
	})
}

func distributedReplicaSettings(preferLocalhostReplica uint8) clickhouse.Settings {
	settings := clickhouse.Settings{
		"prefer_localhost_replica": preferLocalhostReplica,
	}
	if preferLocalhostReplica == 0 {
		// Force ClickHouse to route deterministically to the non-local replica (offset 1,
		// clickhouse-02) instead of random load balancing or hedged requests across both replicas.
		settings["load_balancing"] = "first_or_random"
		settings["load_balancing_first_offset"] = 1
		settings["use_hedged_requests"] = 0
	}
	return settings
}

type measuredTraceSearch struct {
	boundsQuery string
	spanQuery   string
	spanRows    uint64
	traces      []ptrace.Traces
	remoteLogs  []queryLogEntry
}

func (s *ClickHouseStorageIntegration) runMeasuredFindTraces(
	t *testing.T,
	coordinator driver.Conn,
	remoteReplica driver.Conn,
	queryParams tracestore.TraceQueryParams,
	preferLocalhostReplica uint8,
) measuredTraceSearch {
	t.Helper()
	queryID := fmt.Sprintf("jaeger-distributed-test-%d-%d", preferLocalhostReplica, time.Now().UnixNano())
	ctx := clickhouse.Context(
		context.Background(),
		clickhouse.WithQueryID(queryID),
		clickhouse.WithSettings(distributedReplicaSettings(preferLocalhostReplica)),
	)
	traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(s.TraceReader.FindTraces(ctx, queryParams)))
	require.NoError(t, err)
	require.Len(t, traces, 1)

	require.NoError(t, coordinator.Exec(context.Background(), "SYSTEM FLUSH LOGS"))
	require.NoError(t, remoteReplica.Exec(context.Background(), "SYSTEM FLUSH LOGS"))

	res := measuredTraceSearch{
		traces:     traces,
		remoteLogs: readQueryLog(t, remoteReplica, queryID, false),
	}
	for _, entry := range readQueryLog(t, coordinator, queryID, true) {
		switch {
		case strings.Contains(entry.query, "FROM trace_id_timestamps"):
			res.boundsQuery = entry.query
		case strings.Contains(entry.query, "SELECT DISTINCT"):
			// The candidate search is asserted by the corpus itself.
		case strings.Contains(entry.query, "FROM spans") || strings.Contains(entry.query, "spans s"):
			res.spanQuery = entry.query
			res.spanRows += entry.readRows
		default:
		}
	}
	return res
}

func hasQueryTargetingTable(logs []queryLogEntry, table string) bool {
	for _, entry := range logs {
		if strings.Contains(entry.query, table) {
			return true
		}
	}
	return false
}

func buildBoundedTraceInPast(start time.Time) ptrace.Traces {
	trace := ptrace.NewTraces()
	rs := trace.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "bounded-trace-service")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{0xB2})
	span.SetSpanID(pcommon.SpanID{0xB2, 1})
	span.SetName("bounded-past-operation")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Millisecond)))
	return trace
}

// assertRemotePlanPushesTraceIDFilter verifies that the distributed plan delegates reading to
// ReadFromRemote and pushes a trace_id Prewhere/Filter predicate into every ReadFromRemote subtree
// rather than filtering only on the coordinator above an unfiltered remote scan.
func assertRemotePlanPushesTraceIDFilter(t *testing.T, plan string) []string {
	t.Helper()
	subtrees := extractPlanSubtrees(plan, "ReadFromRemote")
	require.NotEmpty(t, subtrees, "distributed plan did not include ReadFromRemote:\n%s", plan)
	for _, subtree := range subtrees {
		assertLeafPlanPushesTraceIDFilter(t, subtree)
	}
	return subtrees
}

// assertLocalPlanPushesTraceIDFilter verifies that local-replica execution reads ReadFromMergeTree
// without ReadFromRemote and pushes the trace_id Prewhere/Filter predicate down.
func assertLocalPlanPushesTraceIDFilter(t *testing.T, plan string) {
	t.Helper()
	require.NotContains(t, plan, "ReadFromRemote",
		"expected local replica execution without ReadFromRemote:\n%s", plan)
	assertLeafPlanPushesTraceIDFilter(t, plan)
}

func assertLeafPlanPushesTraceIDFilter(t *testing.T, planSection string) {
	t.Helper()
	require.Contains(t, planSection, "ReadFromMergeTree",
		"plan section does not read from MergeTree local table:\n%s", planSection)

	hasFilterStepWithTraceID := false
	for _, line := range strings.Split(planSection, "\n") {
		trimmed := strings.TrimSpace(line)
		if (strings.HasPrefix(trimmed, "Prewhere filter column:") || strings.HasPrefix(trimmed, "Filter column:")) &&
			traceIDPredicatePattern.MatchString(trimmed) {
			hasFilterStepWithTraceID = true
			break
		}
	}
	require.True(t, hasFilterStepWithTraceID,
		"plan section has no Prewhere/Filter step filtering on trace_id:\n%s", planSection)
}

// extractPlanSubtrees collects each indented subtree rooted at a step starting with stepPrefix.
// Used with EXPLAIN distributed=1 to isolate remote-node plans beneath ReadFromRemote from any
// coordinator steps sitting at shallower indentation.
func extractPlanSubtrees(plan string, stepPrefix string) []string {
	lines := strings.Split(plan, "\n")
	var subtrees []string
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, stepPrefix) {
			continue
		}
		rootIndent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		subtree := []string{lines[i]}
		j := i + 1
		for ; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				continue
			}
			nextIndent := len(next) - len(strings.TrimLeft(next, " "))
			if nextIndent <= rootIndent {
				break
			}
			subtree = append(subtree, next)
		}
		subtrees = append(subtrees, strings.Join(subtree, "\n"))
		i = j - 1
	}
	return subtrees
}

// assertIndexCondition parses ClickHouse's `EXPLAIN indexes=1` output and checks that one of
// indexNames is active on expectedKey with a real condition (i.e. not `Condition: true`).
func assertIndexCondition(t *testing.T, plan string, expectedKey string, indexNames ...string) {
	t.Helper()
	var section []string
	var matchedName string
	for _, name := range indexNames {
		if s := extractIndexSection(plan, name); len(s) > 0 {
			section = s
			matchedName = name
			break
		}
	}
	require.NotEmpty(t, section, "none of indexes %v found in plan:\n%s", indexNames, plan)

	var hasKey, hasNonTrueCondition bool
	inKeys := false
	for _, line := range section {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Keys:" {
			inKeys = true
			continue
		}
		if inKeys {
			if strings.Contains(trimmed, ":") {
				inKeys = false
			} else if trimmed == expectedKey {
				hasKey = true
			}
		}
		if strings.HasPrefix(trimmed, "Condition:") {
			cond := strings.TrimSpace(strings.TrimPrefix(trimmed, "Condition:"))
			if cond != "" && cond != "true" {
				hasNonTrueCondition = true
			}
		}
	}
	require.True(t, hasKey, "index %q does not use key %q in:\n%s", matchedName, expectedKey, strings.Join(section, "\n"))
	require.True(t, hasNonTrueCondition, "index %q has trivial Condition: true (unfiltered scan) in:\n%s", matchedName, strings.Join(section, "\n"))
}

// extractIndexSection returns the indented lines belonging to indexName inside an `Indexes:` block.
func extractIndexSection(plan string, indexName string) []string {
	lines := strings.Split(plan, "\n")
	inIndexes := false
	indexesIndent := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if trimmed == "Indexes:" {
			inIndexes = true
			indexesIndent = indent
			continue
		}
		if inIndexes && indent <= indexesIndent {
			inIndexes = false
		}
		if !inIndexes || trimmed != indexName {
			continue
		}
		section := []string{line}
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				continue
			}
			nextIndent := len(next) - len(strings.TrimLeft(next, " "))
			if nextIndent <= indent {
				break
			}
			section = append(section, next)
		}
		return section
	}
	return nil
}

type queryLogEntry struct {
	query    string
	readRows uint64
}

func readQueryLog(t *testing.T, conn driver.Conn, queryID string, initialOnly bool) []queryLogEntry {
	t.Helper()
	isInitial := uint8(0)
	if initialOnly {
		isInitial = 1
	}
	rows, err := conn.Query(context.Background(), `
		SELECT query, read_rows
		FROM system.query_log
		WHERE initial_query_id = ? AND type = 'QueryFinish' AND is_initial_query = ?`,
		queryID, isInitial)
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

func countAllSpanRows(t *testing.T, conn driver.Conn) uint64 {
	row := conn.QueryRow(context.Background(), "SELECT count() FROM spans")
	var count uint64
	require.NoError(t, row.Scan(&count))
	return count
}

// explainQuery runs EXPLAIN on query. Setting pretty=0 and compact=0 keeps legacy EXPLAIN output
// (space-indented nodes, Prewhere/Filter column labels, and Indexes blocks) across ClickHouse 26.8+.
// The query string is not user input: it is read back from system.query_log for the test's queryID.
func explainQuery(t *testing.T, conn driver.Conn, query string, distributed bool, preferLocalhostReplica uint8) string {
	t.Helper()
	ctx := clickhouse.Context(
		context.Background(),
		clickhouse.WithSettings(distributedReplicaSettings(preferLocalhostReplica)),
	)
	distFlag := 0
	if distributed {
		distFlag = 1
	}
	explain := fmt.Sprintf(
		"EXPLAIN distributed=%d, pretty=0, compact=0, actions=1, indexes=1, description=1 %s",
		distFlag,
		query,
	)
	rows, err := conn.Query(ctx, explain)
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
