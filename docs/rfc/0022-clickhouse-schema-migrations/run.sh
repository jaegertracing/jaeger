#!/bin/bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# The cluster tests of RFC 0022, Appendix A. The script starts four ClickHouse
# nodes with Docker Compose, runs every test, prints each result under the test
# identifier that the appendix cites, and removes the nodes.
#
# Usage: ./run.sh > results.txt
#        CLICKHOUSE_IMAGE=clickhouse/clickhouse-server:26.9 ./run.sh > results.txt
#
# COMPOSE_PROJECT names the Compose project, so that two runs with different
# names can go on at the same time.

set -u
cd "$(dirname "$0")"
PROJECT=${COMPOSE_PROJECT:-rfc22}
PASSWORD=""
SCRATCH=$(mktemp -d)
# The script removes its temporary files and its containers when it ends,
# also when it is interrupted.
trap 'rm -rf "$SCRATCH"; compose down -v >/dev/null 2>&1' EXIT
trap 'exit 130' INT TERM

compose() { docker compose -p "$PROJECT" "$@"; }

# client NODE [client flags...] runs clickhouse-client on a node.
client() {
  local n=$1; shift
  compose exec -T "ch$n" clickhouse-client --send_logs_level=none ${PASSWORD:+--password "$PASSWORD"} "$@" 2>&1
}

# query NODE SQL [client flags...] prints the result, or the code and the first
# 250 characters of the message when the statement fails.
query() {
  local n=$1 sql=$2 out; shift 2
  out=$(client "$n" "$@" -q "$sql")
  if echo "$out" | grep -q 'Exception'; then
    echo "$out" | grep -o 'Code: [0-9]*\. DB::[A-Za-z]*Exception.\{0,250\}' | head -1 | sed 's/Received from [a-z0-9:]*\. DB::Exception: //'
  else
    [ -z "$out" ] || echo "$out" | head -12
  fi
}

# join_lines joins the rows of a result into one line.
join_lines() { tr '\n' ';' | sed 's/\t/ /g; s/;$//; s/;/; /g'; echo; }

# per_node CLUSTER TABLE EXPR [WHERE] prints EXPR for every node of the cluster.
per_node() {
  query 1 "SELECT hostName() AS h, $3 FROM clusterAllReplicas('$1', $2) ${4:+WHERE $4} GROUP BY h ORDER BY h" | join_lines
}

test_id() { echo; echo "[$1] $2"; }
# result prints one labeled value, without trailing spaces.
result() { echo "    $1: $2" | sed 's/ *$//'; }

wait_node() { until client "$1" -q "SELECT 1" 2>/dev/null | grep -q '^1$'; do sleep 0.5; done; }
# wait_all waits until every node answers and has a Keeper session, and then
# gives the replicated tables five seconds to leave read-only mode.
wait_all() {
  local n
  for n in 1 2 3 4; do
    wait_node "$n"
    until client "$n" -q "SELECT count() FROM system.zookeeper WHERE path = '/'" 2>/dev/null | grep -q '^[0-9]'; do sleep 0.5; done
  done
  sleep 5
}

# wait_writable DB TABLE waits up to 180 seconds until the replicated table
# is out of read-only mode on every node, and prints the seconds it waited.
wait_writable() {
  local start=$SECONDS
  until [ "$(for n in 1 2 3 4; do client "$n" -q "SELECT is_readonly FROM system.replicas WHERE database = '$1' AND table = '$2'" 2>/dev/null; done | tr -d '\n')" = "0000" ]; do
    [ $((SECONDS - start)) -ge 180 ] && break
    sleep 1
  done
  echo "$((SECONDS - start))s"
}

# sync_replicas TABLE [NODE...] makes the nodes, by default all four, copy the
# rows that their shard already holds, so that a count does not depend on
# replication lag.
sync_replicas() {
  local table=$1 n; shift
  [ $# -gt 0 ] || set -- 1 2 3 4
  for n in "$@"; do query "$n" "SYSTEM SYNC REPLICA $table"; done
}

# sync_3a [NODE...] runs sync_replicas on both replicated tables of database s3a.
sync_3a() { sync_replicas s3a.spans_local "$@"; sync_replicas s3a.services_local "$@"; }

# bounded SECONDS NODE SQL [client flags...] runs a statement that may not
# return, and stops the client after SECONDS.
bounded() {
  local limit=$1 n=$2 sql=$3 out status; shift 3
  out=$(compose exec -T "ch$n" timeout "$limit" clickhouse-client --send_logs_level=none "$@" -q "$sql" 2>&1); status=$?
  if [ $status = 124 ]; then
    echo "no answer, client stopped"
  elif echo "$out" | grep -q 'Exception'; then
    echo "$out" | grep -o 'Code: [0-9]*\. DB::[A-Za-z]*Exception.\{0,200\}' | head -1
  else
    echo "returned"
  fi
}

# set_macros MODE writes the shard and replica macros of every node.
#   one_shard   shard1 and replica1 to replica4
#   two_by_two  shard1 on ch1 and ch2, shard2 on ch3 and ch4; replica1 and replica2
#   mismatched  names that match no database
set_macros() {
  local n shard replica
  for n in 1 2 3 4; do
    case $1 in
      one_shard) shard=shard1; replica=replica$n ;;
      two_by_two) shard=shard$(( (n + 1) / 2 )); replica=replica$(( (n + 1) % 2 + 1 )) ;;
      mismatched) shard=other_shard$(( (n + 1) / 2 )); replica=other_replica$n ;;
    esac
    compose exec -T "ch$n" bash -c "printf 'macros:\n  shard: $shard\n  replica: $replica\n' > /etc/clickhouse-server/config.d/macros.yaml"
  done
  compose restart >/dev/null 2>&1
  wait_all
}

start_cluster() { compose down -v >/dev/null 2>&1; compose up -d >/dev/null 2>&1; wait_all; }

DDL_OUTPUT_NONE="--distributed_ddl_output_mode=none"
DDL_OUTPUT_ONLY_ACTIVE="--distributed_ddl_output_mode=throw_only_active"
FOREGROUND_INSERT="--distributed_foreground_insert=1"
# SPAN_ROWS selects 1000 rows in 100 traces of 10 spans.
SPAN_ROWS="SELECT toString(intDiv(number, 10)), concat('svc', toString(number % 7)), now() FROM numbers(1000)"
SPAN_COLUMNS="(trace_id String, service String, timestamp DateTime)"

###############################################################################
echo "# Environment"
start_cluster
result "image" "${CLICKHOUSE_IMAGE:-clickhouse/clickhouse-server:26.3.57}"
result "version" "$(query 1 "SELECT version()")"
result "insert_deduplicate, replicated_deduplication_window" "$(query 1 "SELECT (SELECT value FROM system.settings WHERE name = 'insert_deduplicate'), (SELECT value FROM system.merge_tree_settings WHERE name = 'replicated_deduplication_window')" | join_lines)"
result "distributed_ddl_task_timeout default" "$(query 1 "SELECT value FROM system.settings WHERE name = 'distributed_ddl_task_timeout'")"

###############################################################################
echo; echo "# Part N: no macros on any node"
result "macros on all nodes" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM system.macros"; done | join_lines)"

test_id N1 "Setup 2 without macros: the database fills the path from its own names"
for n in 1 2 3 4; do query $n "CREATE DATABASE s2 ENGINE = Replicated('/clickhouse/databases/s2', 'shard1', 'replica$n')"; done
query 1 "CREATE TABLE s2.spans $SPAN_COLUMNS ENGINE = ReplicatedMergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE
sleep 2
result "system.replicas on every node" "$(per_node s2 system.replicas "any(replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>')), any(replica_name), any(total_replicas)" "database = 's2'")"
result "stored engine" "$(client 1 --format TSVRaw -q "SHOW CREATE TABLE s2.spans" | grep ENGINE)"
query 1 "INSERT INTO s2.spans $SPAN_ROWS"; query 4 "SYSTEM SYNC REPLICA s2.spans"
result "1000 rows inserted on ch1, count on ch4" "$(query 4 "SELECT count() FROM s2.spans")"

test_id N2 "Setup 3b without macros"
for n in 1 2 3 4; do query $n "CREATE DATABASE s3b ENGINE = Replicated('/clickhouse/databases/s3b', 'shard$n', 'replica1')"; done
query 1 "CREATE TABLE spans_local $SPAN_COLUMNS ENGINE = MergeTree ORDER BY trace_id" --database s3b $DDL_OUTPUT_NONE
query 1 "CREATE TABLE spans AS spans_local ENGINE = Distributed('four_by_one', currentDatabase(), 'spans_local', cityHash64(trace_id))" --database s3b $DDL_OUTPUT_NONE
sleep 2
query 3 "INSERT INTO s3b.spans $SPAN_ROWS" $FOREGROUND_INSERT
result "rows in spans_local on every node" "$(per_node four_by_one s3b.spans_local "count()")"
result "count through spans on ch2" "$(query 2 "SELECT count() FROM s3b.spans")"
result "traces on more than one node" "$(query 1 "SELECT count() FROM (SELECT trace_id FROM clusterAllReplicas('four_by_one', s3b.spans_local) GROUP BY trace_id HAVING uniqExact(hostName()) > 1)")"

test_id N3 "An Atomic database refuses a Replicated engine without arguments"
result "CREATE TABLE default.argless (value Int32) ENGINE = ReplicatedMergeTree ORDER BY value" "$(query 1 "CREATE TABLE default.argless (value Int32) ENGINE = ReplicatedMergeTree ORDER BY value")"

test_id N4 "Restart of every node without macros"
compose restart >/dev/null 2>&1; wait_all
result "s2.spans on every node" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM s2.spans"; done | join_lines)"
result "the cause in that message, on ch1" "$(client 1 -q "SELECT count() FROM s2.spans" | grep -o "No macro '[a-z]*' in config" | head -1)"
result "s3b.spans_local on every node" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM s3b.spans_local"; done | join_lines)"

###############################################################################
echo; echo "# Part R: setup 2, one shard of four replicas, macros equal to the database's names"
start_cluster; set_macros one_shard
result "macros" "$(per_node four_by_one system.macros "groupArray(substitution)")"

test_id R1 "The database and its own cluster"
for n in 1 2 3 4; do query $n "CREATE DATABASE s2 ENGINE = Replicated('/clickhouse/databases/s2', 'shard1', 'replica$n')"; done
result "system.clusters for cluster s2, read on ch3 (shard_num, internal_replication, database_shard_name, database_replica_name)" "$(query 3 "SELECT shard_num, internal_replication, database_shard_name, database_replica_name FROM system.clusters WHERE cluster = 's2' ORDER BY database_replica_name" | join_lines)"

test_id R2 "A table created on one node"
query 1 "CREATE TABLE s2.spans $SPAN_COLUMNS ENGINE = ReplicatedMergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE
query 1 "CREATE TABLE s2.services (service String) ENGINE = ReplicatedAggregatingMergeTree ORDER BY service" $DDL_OUTPUT_NONE
sleep 2
result "system.replicas for spans on every node" "$(per_node s2 system.replicas "any(replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>')), any(replica_name), any(total_replicas)" "database = 's2' AND table = 'spans'")"
result "distinct table UUIDs across nodes" "$(query 1 "SELECT uniqExact(uuid) FROM clusterAllReplicas('s2', system.tables) WHERE database = 's2' AND name = 'spans'")"
query 1 "INSERT INTO s2.spans $SPAN_ROWS"; query 4 "SYSTEM SYNC REPLICA s2.spans"
result "1000 rows inserted on ch1, count on ch4" "$(query 4 "SELECT count() FROM s2.spans")"

test_id R3 "Statements that the database refuses"
result "explicit path and replica name" "$(query 1 "CREATE TABLE s2.explicit (value Int32) ENGINE = ReplicatedMergeTree('/clickhouse/tables/x/t', '{replica}') ORDER BY value")"
result "ON CLUSTER" "$(query 2 "CREATE TABLE s2.on_cluster ON CLUSTER two_by_two (value Int32) ENGINE = MergeTree ORDER BY value")"

test_id R4 "Statements that the database delivers, each sent to one node"
query 3 "CREATE MATERIALIZED VIEW s2.services_mv TO s2.services AS SELECT service FROM s2.spans GROUP BY service" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
query 2 "ALTER TABLE s2.spans MODIFY SETTING ttl_only_drop_parts = 1" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
query 3 "ALTER TABLE s2.services_mv MODIFY QUERY SELECT service FROM s2.spans WHERE service != '' GROUP BY service" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
query 4 "ALTER TABLE s2.spans ADD COLUMN IF NOT EXISTS schema_url String" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
query 3 "ALTER TABLE s2.spans ADD INDEX IF NOT EXISTS idx_service service TYPE set(100) GRANULARITY 1" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
query 3 "ALTER TABLE s2.spans MODIFY COLUMN service CODEC(ZSTD(1))" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
query 3 "ALTER TABLE s2.spans MODIFY TTL timestamp + INTERVAL 7 DAY SETTINGS materialize_ttl_after_modify = 0" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
sleep 3
result "MODIFY SETTING in create_table_query" "$(per_node s2 system.tables "any(position(create_table_query, 'ttl_only_drop_parts = 1') > 0)" "database = 's2' AND name = 'spans'")"
result "MODIFY QUERY in create_table_query" "$(per_node s2 system.tables "any(position(create_table_query, 'service != ') > 0)" "database = 's2' AND name = 'services_mv'")"
result "ADD COLUMN in system.columns" "$(per_node s2 system.columns "count()" "database = 's2' AND table = 'spans' AND name = 'schema_url'")"
result "ADD INDEX in system.data_skipping_indices" "$(per_node s2 system.data_skipping_indices "count()" "database = 's2' AND table = 'spans' AND name = 'idx_service'")"
result "CODEC and TTL in create_table_query" "$(per_node s2 system.tables "any(position(create_table_query, 'CODEC(ZSTD(1))') > 0), any(position(create_table_query, 'TTL timestamp') > 0)" "database = 's2' AND name = 'spans'")"
result "rows in system.mutations for spans, all nodes" "$(query 1 "SELECT count() FROM clusterAllReplicas('s2', system.mutations) WHERE database = 's2' AND table = 'spans'")"
result "distinct create_table_query texts across the four nodes (spans, services_mv)" "$(query 1 "SELECT uniqExactIf(create_table_query, name = 'spans'), uniqExactIf(create_table_query, name = 'services_mv') FROM clusterAllReplicas('s2', system.tables) WHERE database = 's2'" | join_lines)"

test_id R5 "What the startup checks read through the database's own cluster"
result "system.macros" "$(per_node s2 system.macros "groupArray((macro, substitution))")"
result "each node's own row of system.clusters" "$(per_node s2 system.clusters "any(database_shard_name), any(database_replica_name)" "is_local AND cluster = 's2'")"
result "system.databases" "$(per_node s2 system.databases "any(engine)" "name = 's2'")"
result "system.server_settings" "$(per_node s2 system.server_settings "groupArray((name, value, changed))" "name IN ('default_replica_path', 'default_replica_name')")"
result "distinct serverUUID()" "$(query 1 "SELECT uniqExact(serverUUID()) FROM clusterAllReplicas('s2', system.one)")"

test_id R6 "A materialized view and rows that a replica copies"
query 1 "CREATE TABLE s2.view_target (trace_id String) ENGINE = MergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE
query 1 "CREATE MATERIALIZED VIEW s2.view_target_mv TO s2.view_target AS SELECT trace_id FROM s2.spans" $DDL_OUTPUT_NONE
sleep 2
query 1 "INSERT INTO s2.spans (trace_id, service, timestamp) SELECT toString(number), 'view', now() FROM numbers(100)"
sync_replicas s2.spans
result "rows with service 'view' in spans on every node" "$(per_node s2 s2.spans "count()" "service = 'view'")"
result "rows in view_target, a table that is not replicated, on every node" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM s2.view_target"; done | join_lines)"

test_id R7 "alter_sync with a replica that does not apply its queue"
query 4 "SYSTEM STOP REPLICATION QUEUES s2.spans"
SECONDS=0
result "ALTER on ch1 with alter_sync = 1" "$(query 1 "ALTER TABLE s2.spans ADD COLUMN IF NOT EXISTS sync1 UInt8" --alter_sync=1 $DDL_OUTPUT_NONE | join_lines) returned after ${SECONDS}s"
result "the column on every node at once" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM system.columns WHERE database = 's2' AND table = 'spans' AND name = 'sync1'"; done | join_lines)"
# The limit of 200 seconds is past distributed_ddl_task_timeout, so the wait is
# not that of the database for its nodes.
SECONDS=0
result "ALTER on ch1 with alter_sync = 2, client stopped after 200 seconds" "$(bounded 200 1 "ALTER TABLE s2.spans ADD COLUMN IF NOT EXISTS sync2 UInt8" --alter_sync=2 $DDL_OUTPUT_ONLY_ACTIVE) after ${SECONDS}s"
query 4 "SYSTEM START REPLICATION QUEUES s2.spans"; sleep 3
result "both columns on ch4 after the queue restarted" "$(query 4 "SELECT count() FROM system.columns WHERE database = 's2' AND table = 'spans' AND name IN ('sync1', 'sync2')")"

test_id R8 "A mutation with mutations_sync = 2 and alter_sync = 2"
SECONDS=0
result "MODIFY COLUMN service LowCardinality(String) on ch1" "$(query 1 "ALTER TABLE s2.spans MODIFY COLUMN service LowCardinality(String) CODEC(ZSTD(1))" --mutations_sync=2 --alter_sync=2 $DDL_OUTPUT_ONLY_ACTIVE | join_lines)"
result "mutations of spans on all nodes right after the statement returned (${SECONDS}s): total, unfinished" "$(query 1 "SELECT count(), countIf(NOT is_done) FROM clusterAllReplicas('s2', system.mutations) WHERE database = 's2' AND table = 'spans'" | join_lines)"
result "type on every node" "$(per_node s2 system.columns "any(type)" "database = 's2' AND table = 'spans' AND name = 'service'")"

test_id R9 "Two connections issuing the same statements at once"
# Each loop runs inside its container, so that the two loops issue their
# statements without the delay of starting a client from the host.
for n in 1 2; do
  compose exec -T "ch$n" bash -c 'date +%s; for k in $(seq 1 50); do clickhouse-client --alter_sync=1 --distributed_ddl_output_mode=throw_only_active -q "ALTER TABLE s2.spans ADD COLUMN IF NOT EXISTS race$k UInt8" 2>&1 | grep -o "Code: [0-9]*"; done; date +%s' > "$SCRATCH/race-$n.txt" &
done
wait
result "seconds from the start of the first loop: start and end of the loop on ch1, start and end of the loop on ch2" "$(cat $SCRATCH/race-1.txt $SCRATCH/race-2.txt | grep -v Code | sort -n | head -1 > $SCRATCH/race-zero; for n in 1 2; do grep -v Code $SCRATCH/race-$n.txt | while read -r t; do echo $((t - $(cat $SCRATCH/race-zero))); done; done | join_lines)"
result "error codes seen by the two loops over 50 statements each" "$(cat $SCRATCH/race-1.txt $SCRATCH/race-2.txt | grep Code | sort | uniq -c | join_lines)"
result "race columns on every node" "$(per_node s2 system.columns "count()" "database = 's2' AND table = 'spans' AND name LIKE 'race%'")"
rm -f $SCRATCH/race-*

test_id R10 "A user without the REMOTE grant"
for n in 1 2 3 4; do query $n "CREATE USER limited"; query $n "GRANT SELECT ON *.* TO limited"; done
result "clusterAllReplicas as that user" "$(query 1 "SELECT count() FROM clusterAllReplicas('s2', system.one)" --user limited)"
query 1 "GRANT REMOTE ON *.* TO limited"
result "after GRANT REMOTE ON *.*" "$(query 1 "SELECT count() FROM clusterAllReplicas('s2', system.one)" --user limited)"

test_id R11 "One shard of four replicas under a Distributed table without internal_replication"
query 1 "CREATE TABLE s2.four_local (trace_id String) ENGINE = ReplicatedMergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE
query 1 "CREATE TABLE s2.four AS s2.four_local ENGINE = Distributed('one_by_four_no_internal_replication', currentDatabase(), 'four_local')" $DDL_OUTPUT_NONE
sleep 2
query 1 "INSERT INTO s2.four SELECT toString(number) FROM numbers(1000)" $FOREGROUND_INSERT
sync_replicas s2.four_local
result "rows in four_local on every node, 1000 inserted" "$(per_node s2 s2.four_local "count()")"
query 1 "TRUNCATE TABLE s2.four_local" $DDL_OUTPUT_NONE; sleep 2
query 1 "INSERT INTO s2.four SELECT toString(number) FROM numbers(1000)" $FOREGROUND_INSERT --insert_deduplicate=0
sync_replicas s2.four_local
result "the same with insert_deduplicate = 0" "$(per_node s2 s2.four_local "count()")"

test_id R12 "Restart of every node with the macros defined"
compose restart >/dev/null 2>&1; wait_all
result "is_readonly of spans on every node, five seconds after every node has a Keeper session" "$(per_node s2 system.replicas "any(is_readonly)" "database = 's2' AND table = 'spans'")"
result "wait until spans is writable on every node" "$(wait_writable s2 spans)"
result "path, is_readonly and total_replicas of spans on every node" "$(per_node s2 system.replicas "any(replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>')), any(is_readonly), any(total_replicas)" "database = 's2' AND table = 'spans'")"
query 3 "INSERT INTO s2.spans (trace_id, service, timestamp) VALUES ('after-restart', 'x', now())"; query 2 "SYSTEM SYNC REPLICA s2.spans"
result "a row inserted on ch3, read on ch2" "$(query 2 "SELECT count() FROM s2.spans WHERE trace_id = 'after-restart'")"

test_id R13 "A node that is paused, which ClickHouse still lists as active for a while"
compose pause ch4 >/dev/null 2>&1
for k in 1 2; do
  SECONDS=0
  result "statement $k with throw_only_active" "$(query 1 "ALTER TABLE s2.spans ADD COLUMN IF NOT EXISTS paused$k UInt8" $DDL_OUTPUT_ONLY_ACTIVE | join_lines) after ${SECONDS}s"
done
SECONDS=0
result "clusterAllReplicas with skip_unavailable_shards = 1" "$(query 1 "SELECT count() FROM clusterAllReplicas('s2', system.one)" --skip_unavailable_shards=1) rows after ${SECONDS}s"
compose unpause ch4 >/dev/null 2>&1

###############################################################################
echo; echo "# Part S: two shards of two replicas, macros equal to the database's names"
start_cluster; set_macros two_by_two
result "macros" "$(per_node four_by_one system.macros "groupArray(substitution)")"

test_id S1 "Setup 3a"
for n in 1 2 3 4; do query $n "CREATE DATABASE s3a ENGINE = Replicated('/clickhouse/databases/s3a', 'shard$(( (n + 1) / 2 ))', 'replica$(( (n + 1) % 2 + 1 ))')"; done
query 1 "CREATE TABLE spans_local $SPAN_COLUMNS ENGINE = ReplicatedMergeTree ORDER BY trace_id" --database s3a $DDL_OUTPUT_NONE
query 1 "CREATE TABLE spans AS spans_local ENGINE = Distributed('two_by_two', currentDatabase(), 'spans_local', cityHash64(trace_id))" --database s3a $DDL_OUTPUT_NONE
query 1 "CREATE TABLE s3a.spans_from_default AS s3a.spans_local ENGINE = Distributed('two_by_two', currentDatabase(), 'spans_local', cityHash64(trace_id))" --database default $DDL_OUTPUT_NONE
query 1 "CREATE TABLE s3a.services_local (service String) ENGINE = ReplicatedAggregatingMergeTree ORDER BY service" $DDL_OUTPUT_NONE
query 1 "CREATE TABLE s3a.services AS s3a.services_local ENGINE = Distributed('two_by_two', currentDatabase(), 'services_local')" $DDL_OUTPUT_NONE
query 1 "CREATE MATERIALIZED VIEW s3a.services_mv TO s3a.services_local AS SELECT service FROM s3a.spans_local GROUP BY service" $DDL_OUTPUT_NONE
sleep 3
result "tables on every node" "$(per_node two_by_two system.tables "count()" "database = 's3a'")"
result "system.replicas for spans_local" "$(per_node two_by_two system.replicas "any(replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>')), any(replica_name), any(total_replicas)" "database = 's3a' AND table = 'spans_local'")"
result "engine_full of spans, created with unqualified names on a connection to database s3a, read on ch4" "$(query 4 "SELECT engine_full FROM system.tables WHERE database = 's3a' AND name = 'spans'")"
result "engine_full of spans_from_default, created with qualified names on a connection to database default" "$(query 4 "SELECT engine_full FROM system.tables WHERE database = 's3a' AND name = 'spans_from_default'")"

test_id S2 "A foreground insert through the Distributed table"
query 2 "INSERT INTO s3a.spans $SPAN_ROWS" $FOREGROUND_INSERT
result "rows in spans_local on every node, read at once without SYSTEM SYNC REPLICA" "$(per_node two_by_two s3a.spans_local "count()")"
sync_3a 1 2 3 4
result "count through spans on ch3, after SYSTEM SYNC REPLICA on every node" "$(query 3 "SELECT count() FROM s3a.spans")"
result "traces on more than one shard" "$(query 1 "SELECT count() FROM (SELECT trace_id FROM clusterAllReplicas('two_by_two', s3a.spans_local) GROUP BY trace_id HAVING uniqExact(hostName() IN ('ch1', 'ch2')) > 1)")"
result "distinct services through the Distributed table, 7 inserted" "$(query 1 "SELECT uniqExact(service) FROM s3a.services")"

test_id S3 "A background insert, the ClickHouse default"
query 2 "INSERT INTO s3a.spans $SPAN_ROWS; SELECT count() FROM s3a.spans" --distributed_foreground_insert=0 > $SCRATCH/background.txt
result "count through spans in the same connection right after the INSERT returned, 2000 expected" "$(join_lines < $SCRATCH/background.txt)"
sleep 4; rm -f $SCRATCH/background.txt; sync_3a 1 2 3 4
result "four seconds later, after SYSTEM SYNC REPLICA on every node" "$(query 2 "SELECT count() FROM s3a.spans")"

test_id S4 "A column added to the storage table and then to the Distributed table"
query 1 "ALTER TABLE s3a.spans_local ADD COLUMN schema_url String" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
result "SELECT schema_url FROM s3a.spans" "$(query 3 "SELECT count(schema_url) FROM s3a.spans")"
query 1 "ALTER TABLE s3a.spans ADD COLUMN schema_url String" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
sync_3a 1 2 3 4
result "the column of the Distributed table on every node" "$(per_node two_by_two system.columns "count()" "database = 's3a' AND table = 'spans' AND name = 'schema_url'")"
result "the same SELECT, on ch3" "$(query 3 "SELECT count(schema_url) FROM s3a.spans")"

test_id S5 "internal_replication off: 1000 rows through each Distributed table, storage table emptied first"
for cluster in two_by_two_no_internal_replication s3a; do
  query 1 "TRUNCATE TABLE s3a.spans_local" $DDL_OUTPUT_NONE; sleep 2
  query 1 "CREATE TABLE s3a.spans_via_$cluster AS s3a.spans_local ENGINE = Distributed('$cluster', currentDatabase(), 'spans_local', cityHash64(trace_id))" $DDL_OUTPUT_NONE; sleep 2
  query 2 "INSERT INTO s3a.spans_via_$cluster (trace_id, service, timestamp) $SPAN_ROWS" $FOREGROUND_INSERT
  sync_replicas s3a.spans_local
  result "cluster $cluster, internal_replication $(query 1 "SELECT any(internal_replication) FROM system.clusters WHERE cluster = '$cluster'"): rows in spans_local on every node" "$(per_node two_by_two s3a.spans_local "count()")"
  result "cluster $cluster: count through spans" "$(query 1 "SELECT count() FROM s3a.spans")"
done
query 1 "TRUNCATE TABLE s3a.spans_local" $DDL_OUTPUT_NONE; sleep 2
query 2 "INSERT INTO s3a.spans (trace_id, service, timestamp) $SPAN_ROWS" $FOREGROUND_INSERT
sync_replicas s3a.spans_local
result "cluster two_by_two, internal_replication 1, for comparison" "$(per_node two_by_two s3a.spans_local "count()")"

test_id S6 "The internal_replication setting of a Replicated database"
for n in 1 2 3 4; do query $n "CREATE DATABASE s3own ENGINE = Replicated('/clickhouse/databases/s3own', 'shard$(( (n + 1) / 2 ))', 'replica$(( (n + 1) % 2 + 1 ))') SETTINGS internal_replication = 1"; done
result "system.clusters for cluster s3own (shard_num, internal_replication)" "$(query 1 "SELECT shard_num, internal_replication FROM system.clusters WHERE cluster = 's3own' ORDER BY shard_num, replica_num" | join_lines)"
query 1 "CREATE TABLE spans_local $SPAN_COLUMNS ENGINE = ReplicatedMergeTree ORDER BY trace_id" --database s3own $DDL_OUTPUT_NONE
query 1 "CREATE TABLE spans AS spans_local ENGINE = Distributed('s3own', currentDatabase(), 'spans_local', cityHash64(trace_id))" --database s3own $DDL_OUTPUT_NONE
sleep 3
query 2 "INSERT INTO s3own.spans $SPAN_ROWS" $FOREGROUND_INSERT
sync_replicas s3own.spans_local
result "rows in spans_local on every node, 1000 inserted through the database's own cluster" "$(per_node two_by_two s3own.spans_local "count()")"
result "count through spans" "$(query 3 "SELECT count() FROM s3own.spans")"
result "traces on more than one shard" "$(query 1 "SELECT count() FROM (SELECT trace_id FROM clusterAllReplicas('two_by_two', s3own.spans_local) GROUP BY trace_id HAVING uniqExact(hostName() IN ('ch1', 'ch2')) > 1)")"

test_id S7 "A named cluster that divides the replicas of a shard"
query 1 "CREATE TABLE s3a.divided_local (trace_id String) ENGINE = ReplicatedMergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE
query 1 "CREATE TABLE s3a.divided AS s3a.divided_local ENGINE = Distributed('four_by_one', currentDatabase(), 'divided_local', cityHash64(trace_id))" $DDL_OUTPUT_NONE
sleep 2
query 2 "INSERT INTO s3a.divided SELECT toString(number) FROM numbers(1000)" $FOREGROUND_INSERT
sync_replicas s3a.divided_local
result "rows in divided_local on every node, 1000 inserted" "$(per_node two_by_two s3a.divided_local "count()")"
result "count through divided" "$(query 1 "SELECT count() FROM s3a.divided")"
result "each node's shard in four_by_one" "$(per_node two_by_two system.clusters "any(shard_num)" "is_local AND cluster = 'four_by_one'")"

test_id S8 "A database whose shard names differ from the macros"
for n in 1 2 3 4; do query $n "CREATE DATABASE one_shard_db ENGINE = Replicated('/clickhouse/databases/one_shard_db', 'shard1', 'replica$n')"; done
query 1 "CREATE TABLE one_shard_db.spans_local (trace_id String) ENGINE = ReplicatedMergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE; sleep 2
result "database_shard_name of every node" "$(per_node two_by_two system.clusters "any(database_shard_name)" "is_local AND cluster = 'one_shard_db'")"
result "Keeper path and replica name of the table on every node" "$(per_node two_by_two system.replicas "any(replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>')), any(replica_name)" "database = 'one_shard_db'")"
query 1 "ALTER TABLE one_shard_db.spans_local ADD COLUMN IF NOT EXISTS extra UInt8" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null; sleep 3
result "a column added on ch1 through that database, on every node" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM system.columns WHERE database = 'one_shard_db' AND table = 'spans_local' AND name = 'extra'"; done | join_lines)"

test_id S9 "What the startup checks read through the named cluster"
result "each node's shard in the named cluster and in the database" "$(per_node two_by_two system.clusters "anyIf(shard_num, cluster = 'two_by_two'), anyIf(internal_replication, cluster = 'two_by_two'), anyIf(database_shard_name, cluster = 's3a'), anyIf(database_replica_name, cluster = 's3a')" "is_local AND cluster IN ('two_by_two', 's3a')")"
result "system.macros" "$(per_node two_by_two system.macros "groupArray((macro, substitution))")"
result "distinct definitions of two_by_two held by the four nodes" "$(query 1 "SELECT uniqExact(definition) FROM (SELECT hostName() AS h, groupArray((shard_num, shard_weight, replica_num, host_name)) AS definition FROM (SELECT * FROM clusterAllReplicas('two_by_two', system.clusters) WHERE cluster = 'two_by_two' ORDER BY shard_num, replica_num) GROUP BY h)")"
result "serverUUID() sets through the named cluster and the database's own cluster are equal" "$(query 1 "SELECT (SELECT arraySort(groupArray(serverUUID())) FROM clusterAllReplicas('two_by_two', system.one)) = (SELECT arraySort(groupArray(serverUUID())) FROM clusterAllReplicas('s3a', system.one))")"
result "system.databases" "$(per_node two_by_two system.databases "any(engine)" "name = 's3a'")"
result "system.server_settings" "$(per_node two_by_two system.server_settings "groupArray((name, changed))" "name IN ('default_replica_path', 'default_replica_name')")"
result "a cluster that does not exist, in system.clusters" "$(query 1 "SELECT count() FROM system.clusters WHERE cluster = 'no_such_cluster'")"
result "a Distributed table over it" "$(query 1 "CREATE TABLE s3a.missing AS s3a.spans_local ENGINE = Distributed('no_such_cluster', currentDatabase(), 'spans_local')" $DDL_OUTPUT_NONE)"
result "a cluster name with a quote" "$(query 1 "CREATE TABLE s3a.quoted AS s3a.spans_local ENGINE = Distributed('two_by_two'', currentDatabase(), 'spans_local')" $DDL_OUTPUT_NONE)"
result "a cluster name that ends with a backslash" "$(query 1 "CREATE TABLE s3a.quoted AS s3a.spans_local ENGINE = Distributed('two_by_two\\', currentDatabase(), 'spans_local')" $DDL_OUTPUT_NONE)"

test_id S10 "The ledger as a MergeTree table, and a Distributed table with an empty database argument"
query 1 "CREATE TABLE s3a.ledger (version UInt32) ENGINE = MergeTree ORDER BY version" $DDL_OUTPUT_NONE; sleep 2
query 3 "INSERT INTO s3a.ledger VALUES (2)"
result "rows read on ch1 locally, and through clusterAllReplicas('s3a', ...)" "$(query 1 "SELECT (SELECT count() FROM s3a.ledger), (SELECT count() FROM (SELECT DISTINCT version FROM clusterAllReplicas('s3a', s3a.ledger)))" | join_lines)"
query 1 "CREATE TABLE s3a.spans_empty_database AS s3a.spans_local ENGINE = Distributed('two_by_two', '', 'spans_local', cityHash64(trace_id))" $DDL_OUTPUT_NONE; sleep 2
result "count through it while default.spans_local does not exist" "$(query 1 "SELECT count() FROM s3a.spans_empty_database")"
for n in 1 2 3 4; do query $n "CREATE TABLE default.spans_local $SPAN_COLUMNS ENGINE = MergeTree ORDER BY trace_id"; done
result "count through it after an empty default.spans_local exists on every node; s3a.spans_local holds $(query 1 "SELECT count() FROM s3a.spans")" "$(query 1 "SELECT count() FROM s3a.spans_empty_database")"

test_id S11 "An Atomic database: statements on ch1, read on ch2"
for n in 1 2; do
  query $n "CREATE TABLE default.plain (value Int32) ENGINE = MergeTree ORDER BY value"
  query $n "CREATE TABLE default.replicated (value Int32, label String) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/{database}/{table}', '{replica}') ORDER BY value"
  query $n "CREATE TABLE default.view_target (value Int32) ENGINE = MergeTree ORDER BY value"
  query $n "CREATE MATERIALIZED VIEW default.replicated_mv TO default.view_target AS SELECT value FROM default.replicated"
done
query 1 "CREATE TABLE default.only_on_ch1 (value Int32) ENGINE = MergeTree ORDER BY value"
query 1 "INSERT INTO default.replicated VALUES (1, 'x')"; query 2 "SYSTEM SYNC REPLICA default.replicated"
result "a row inserted on ch1 into the replicated table, count on ch2, total_replicas" "$(query 2 "SELECT (SELECT count() FROM default.replicated), (SELECT total_replicas FROM system.replicas WHERE database = 'default' AND table = 'replicated')" | join_lines)"
query 1 "ALTER TABLE default.plain ADD COLUMN extra UInt8"
query 1 "ALTER TABLE default.replicated ADD COLUMN extra UInt8"
query 1 "ALTER TABLE default.replicated ADD INDEX idx_label label TYPE set(100) GRANULARITY 1"
query 1 "ALTER TABLE default.replicated MODIFY COLUMN label CODEC(ZSTD(1))"
query 1 "ALTER TABLE default.replicated MODIFY SETTING ttl_only_drop_parts = 1"
query 1 "ALTER TABLE default.replicated_mv MODIFY QUERY SELECT value FROM default.replicated WHERE value > 0"
sleep 3
# select_on_ch2 EXPRESSION prints the value of a SELECT on ch2.
select_on_ch2() { query 2 "SELECT $1"; }
result "distinct UUIDs of the replicated table on ch1 and ch2" "$(for n in 1 2; do query $n "SELECT uuid FROM system.tables WHERE database = 'default' AND name = 'replicated'"; done | sort -u | wc -l | tr -d ' ')"
result "CREATE TABLE only_on_ch1, tables of that name on ch2" "$(select_on_ch2 "count() FROM system.tables WHERE database = 'default' AND name = 'only_on_ch1'")"
result "MergeTree table: ADD COLUMN on ch2" "$(select_on_ch2 "count() FROM system.columns WHERE database = 'default' AND table = 'plain' AND name = 'extra'")"
result "replicated table: ADD COLUMN on ch2" "$(select_on_ch2 "count() FROM system.columns WHERE database = 'default' AND table = 'replicated' AND name = 'extra'")"
result "replicated table: ADD INDEX on ch2" "$(select_on_ch2 "count() FROM system.data_skipping_indices WHERE database = 'default' AND table = 'replicated' AND name = 'idx_label'")"
result "replicated table: CODEC on ch2" "$(select_on_ch2 "position(create_table_query, 'CODEC(ZSTD(1))') > 0 FROM system.tables WHERE database = 'default' AND name = 'replicated'")"
result "replicated table: MODIFY SETTING on ch2" "$(select_on_ch2 "position(create_table_query, 'ttl_only_drop_parts') > 0 FROM system.tables WHERE database = 'default' AND name = 'replicated'")"
result "view: MODIFY QUERY on ch2" "$(select_on_ch2 "position(create_table_query, 'value > 0') > 0 FROM system.tables WHERE database = 'default' AND name = 'replicated_mv'")"

test_id S12 "The order of the SETTINGS clause in create_table_query"
query 1 "CREATE TABLE default.order1 (value Int32) ENGINE = MergeTree ORDER BY value SETTINGS index_granularity = 8192"
query 1 "ALTER TABLE default.order1 MODIFY SETTING ttl_only_drop_parts = 1"
query 1 "CREATE TABLE default.order2 (value Int32) ENGINE = MergeTree ORDER BY value SETTINGS ttl_only_drop_parts = 1, index_granularity = 8192"
for t in order1 order2; do result "$t" "$(query 1 "SELECT extract(create_table_query, 'SETTINGS .*') FROM system.tables WHERE database = 'default' AND name = '$t'")"; done

test_id S13 "A node that is stopped"
compose stop ch4 >/dev/null 2>&1
for k in 1 2 3; do
  SECONDS=0
  result "statement $k on s3a with throw_only_active" "$(query 1 "ALTER TABLE s3a.spans_local ADD COLUMN IF NOT EXISTS down$k UInt8" $DDL_OUTPUT_ONLY_ACTIVE | join_lines) after ${SECONDS}s"
done
SECONDS=0
result "a statement with alter_sync = 2 and throw_only_active, client stopped after 60 seconds" "$(bounded 60 1 "ALTER TABLE s3a.spans_local ADD COLUMN IF NOT EXISTS unfinished UInt8" --alter_sync=2 $DDL_OUTPUT_ONLY_ACTIVE) after ${SECONDS}s"
SECONDS=0
result "a statement with distributed_ddl_output_mode = none" "$(query 1 "ALTER TABLE s3a.spans_local ADD COLUMN IF NOT EXISTS down5 UInt8" $DDL_OUTPUT_NONE | tail -1) after ${SECONDS}s"
for k in $(seq 6 150); do query 1 "ALTER TABLE s3a.spans_local ADD COLUMN IF NOT EXISTS down$k UInt8" $DDL_OUTPUT_ONLY_ACTIVE >/dev/null 2>&1; done
result "statements 5 to 150 queued for ch4" "columns named down* on ch1: $(query 1 "SELECT count() FROM system.columns WHERE database = 's3a' AND table = 'spans_local' AND name LIKE 'down%'")"
SECONDS=0
result "a statement with throw and distributed_ddl_task_timeout = 20" "$(query 1 "ALTER TABLE s3a.spans_local ADD COLUMN IF NOT EXISTS down4 UInt8" --distributed_ddl_output_mode=throw --distributed_ddl_task_timeout=20 | tail -1) after ${SECONDS}s"
result "is_active of ch4 in system.clusters on ch1" "$(query 1 "SELECT is_active FROM system.clusters WHERE cluster = 's3a' AND host_name = 'ch4'")"
result "clusterAllReplicas('s3a', system.one) with skip_unavailable_shards = 1" "$(query 1 "SELECT count() FROM clusterAllReplicas('s3a', system.one)" --skip_unavailable_shards=1) rows"
sync_3a 1 2 3
count_before=$(query 1 "SELECT count() FROM s3a.spans")
query 1 "INSERT INTO s3a.spans (trace_id, service, timestamp) $SPAN_ROWS" $FOREGROUND_INSERT
sync_3a 1 2 3
result "setup 3a: foreground insert of 1000 rows, count through spans before and after" "$count_before; $(query 1 "SELECT count() FROM s3a.spans")"

test_id S14 "The stopped node returns"
COUNT_DOWN="SELECT count() FROM system.columns WHERE database = 's3a' AND table = 'spans_local' AND name LIKE 'down%'"
INSERT_DOWN="INSERT INTO s3a.spans_local (trace_id, down150) VALUES ('early', 1)"
compose start ch4 >/dev/null 2>&1; SECONDS=0
# The loop runs inside the container so that the first read follows the first
# accepted connection without the delay of starting a client from the host.
first_answer=$(compose exec -T ch4 bash -c "until clickhouse-client -q \"$COUNT_DOWN\" 2>/dev/null; do sleep 0.05; done; clickhouse-client -q \"$INSERT_DOWN\" 2>&1 | grep -m1 Code | cut -c1-150; echo '(end)'")
result "ch4 answers after ${SECONDS}s; queued columns present at its first answer, of 150" "$(echo "$first_answer" | head -1)"
result "an insert on ch4 that names the last queued column, right after that answer" "$(echo "$first_answer" | tail -n +2 | join_lines)"
for pause_seconds in 1 2 4 8; do sleep $pause_seconds; result "queued columns present ${SECONDS}s after the start" "$(query 4 "$COUNT_DOWN")"; done
result "the same insert, ${SECONDS}s after the start" "$(query 4 "INSERT INTO s3a.spans_local (trace_id, down150) VALUES ('late', 1)"; echo "(end)")"
query 4 "SYSTEM SYNC REPLICA s3a.spans_local"
result "rows in spans_local on ch3 and ch4" "$(for n in 3 4; do query $n "SELECT count() FROM s3a.spans_local"; done | join_lines)"

test_id S15 "Macros that differ from the database's names"
set_macros mismatched
result "wait for spans_local, created earlier, to become writable, limit 180s" "$(wait_writable s3a spans_local)"
result "spans_local created earlier, on every node: path, replica name, is_readonly, total_replicas" "$(for n in 1 2 3 4; do query $n "SELECT replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>'), replica_name, is_readonly, total_replicas FROM system.replicas WHERE database = 's3a' AND table = 'spans_local'"; done | join_lines)"
query 1 "CREATE TABLE s3a.created_under_other (value Int32) ENGINE = ReplicatedMergeTree ORDER BY value" $DDL_OUTPUT_NONE; sleep 2
result "a table created now: path, replica name, is_readonly on ch1" "$(query 1 "SELECT replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>'), replica_name, is_readonly FROM system.replicas WHERE database = 's3a' AND table = 'created_under_other'" | join_lines)"
set_macros two_by_two
result "macros restored: wait until spans_local is writable on every node" "$(wait_writable s3a spans_local)"
result "macros restored: is_readonly of spans_local and of created_under_other on every node" "$(for n in 1 2 3 4; do query $n "SELECT groupArray((table, is_readonly)) FROM (SELECT table, is_readonly FROM system.replicas WHERE database = 's3a' AND table IN ('spans_local', 'created_under_other') ORDER BY table)"; done | join_lines)"

test_id S16 "Statements on the storage table that are not schema changes, sent to ch1"
for n in 1 2 3 4; do query $n "CREATE DATABASE s3c ENGINE = Replicated('/clickhouse/databases/s3c', 'shard$(( (n + 1) / 2 ))', 'replica$(( (n + 1) % 2 + 1 ))')"; done
query 1 "CREATE TABLE events_local (trace_id String, day Date, value UInt8) ENGINE = ReplicatedMergeTree PARTITION BY day ORDER BY trace_id" --database s3c $DDL_OUTPUT_NONE
query 1 "CREATE TABLE events AS events_local ENGINE = Distributed('two_by_two', currentDatabase(), 'events_local', cityHash64(trace_id))" --database s3c $DDL_OUTPUT_NONE
sleep 3
query 2 "INSERT INTO s3c.events SELECT toString(number), toDate('2026-01-01'), 0 FROM numbers(1000)" $FOREGROUND_INSERT; sync_replicas s3c.events_local
result "rows on every node, 1000 inserted through the Distributed table" "$(per_node two_by_two s3c.events_local "count()")"
query 1 "ALTER TABLE s3c.events_local UPDATE value = sleepEachRow(0.005) WHERE 1" --mutations_sync=0 $DDL_OUTPUT_ONLY_ACTIVE >/dev/null
SECONDS=0
result "ADD COLUMN on ch1 right after a mutation that sleeps about 2.7 seconds on its shard was started" "$(query 1 "ALTER TABLE s3c.events_local ADD COLUMN IF NOT EXISTS during UInt8" --alter_sync=1 $DDL_OUTPUT_ONLY_ACTIVE | join_lines) after ${SECONDS}s"
result "unfinished mutations on ch1 when it returned" "$(query 1 "SELECT countIf(NOT is_done) FROM system.mutations WHERE database = 's3c' AND table = 'events_local'")"
sleep 5
result "ALTER TABLE events_local UPDATE value = 1 WHERE 1, with mutations_sync = 2" "$(query 1 "ALTER TABLE s3c.events_local UPDATE value = 1 WHERE 1" --mutations_sync=2 $DDL_OUTPUT_ONLY_ACTIVE | join_lines)"
sync_replicas s3c.events_local
result "rows with value = 1 on every node" "$(per_node two_by_two s3c.events_local "countIf(value = 1)")"
query 1 "INSERT INTO s3c.events_local (trace_id, day, value) SELECT concat('copy-', trace_id), day, 2 FROM s3c.events_local"; sync_replicas s3c.events_local
result "INSERT INTO events_local SELECT ... FROM events_local: rows with value = 2 on every node" "$(per_node two_by_two s3c.events_local "countIf(value = 2)")"
result "ALTER TABLE events_local DROP PARTITION '2026-01-01'" "$(query 1 "ALTER TABLE s3c.events_local DROP PARTITION '2026-01-01'" $DDL_OUTPUT_ONLY_ACTIVE | join_lines)"
sync_replicas s3c.events_local
result "rows on every node after it" "$(per_node two_by_two s3c.events_local "count()")"

###############################################################################
echo; echo "# Part U: setup 3b, four shards of one node, no macros"
start_cluster
for n in 1 2 3 4; do query $n "CREATE DATABASE s3b ENGINE = Replicated('/clickhouse/databases/s3b', 'shard$n', 'replica1')"; done
query 1 "CREATE TABLE spans_local $SPAN_COLUMNS ENGINE = MergeTree ORDER BY trace_id" --database s3b $DDL_OUTPUT_NONE
query 1 "CREATE TABLE spans AS spans_local ENGINE = Distributed('four_by_one', currentDatabase(), 'spans_local', cityHash64(trace_id))" --database s3b $DDL_OUTPUT_NONE
sleep 2

test_id U1 "A node that is stopped"
compose stop ch4 >/dev/null 2>&1
result "a query through the Distributed table" "$(query 1 "SELECT count() FROM s3b.spans")"
result "foreground insert of rows that all belong to the three live nodes" "$(query 1 "INSERT INTO s3b.spans SELECT toString(number), 'live', now() FROM numbers(1000) WHERE cityHash64(toString(number)) % 4 != 3" $FOREGROUND_INSERT; echo ok)"
result "its rows on the live nodes" "$(for n in 1 2 3; do query $n "SELECT count() FROM s3b.spans_local WHERE service = 'live'"; done | join_lines)"
result "foreground insert of 1000 rows for all four nodes" "$(query 1 "INSERT INTO s3b.spans SELECT toString(number), 'all', now() FROM numbers(1000)" $FOREGROUND_INSERT)"
result "its rows on the live nodes" "$(for n in 1 2 3; do query $n "SELECT count() FROM s3b.spans_local WHERE service = 'all'"; done | join_lines)"
SECONDS=0
result "ALTER through the database with throw_only_active" "$(query 1 "ALTER TABLE s3b.spans_local ADD COLUMN IF NOT EXISTS down1 UInt8" $DDL_OUTPUT_ONLY_ACTIVE | join_lines) after ${SECONDS}s"
compose start ch4 >/dev/null 2>&1; wait_node 4
result "the queued column on ch4 at its first answer" "$(query 4 "SELECT count() FROM system.columns WHERE database = 's3b' AND table = 'spans_local' AND name = 'down1'")"

test_id U2 "A node that is killed during an insert"
( sleep 3; compose kill ch4 >/dev/null 2>&1 ) &
SECONDS=0
result "foreground insert of 200 million rows, with ch4 killed three seconds after its start" "$(query 1 "INSERT INTO s3b.spans SELECT toString(number), 'killed', now() FROM numbers(200000000)" $FOREGROUND_INSERT; echo "(end after ${SECONDS}s)")"
wait
result "its rows on the live nodes" "$(for n in 1 2 3; do query $n "SELECT count() FROM s3b.spans_local WHERE service = 'killed'"; done | join_lines)"

###############################################################################
echo; echo "# Part C: credentials, a changed replica path, clusters that differ between nodes"
start_cluster
for n in 1 2 3 4; do
  compose exec -T "ch$n" bash -c "printf 'users:\n  default:\n    password: test_password\n' > /etc/clickhouse-server/users.d/password.yaml"
  compose exec -T "ch$n" bash -c "printf 'named_collections:\n  cluster_credentials:\n    cluster_username: default\n    cluster_password: test_password\ndefault_replica_path: /custom/{uuid}/{shard}\ndefault_replica_name: custom-{replica}\n' > /etc/clickhouse-server/config.d/part_c.yaml"
done
# ch4 holds a definition of the cluster 'skewed' whose shards are in the other order.
for n in 1 2 3 4; do
  if [ "$n" = 4 ]; then first_shard_host=ch2; second_shard_host=ch1; else first_shard_host=ch1; second_shard_host=ch2; fi
  compose exec -T "ch$n" bash -c "printf 'remote_servers:\n  skewed:\n    shard:\n      - replica: [{host: $first_shard_host, port: 9000, user: default, password: test_password}]\n      - replica: [{host: $second_shard_host, port: 9000, user: default, password: test_password}]\n' > /etc/clickhouse-server/config.d/skewed.yaml"
done
PASSWORD=test_password
set_macros two_by_two

test_id C1 "A password on the default user"
for n in 1 2 3 4; do query $n "CREATE DATABASE plain_db ENGINE = Replicated('/clickhouse/databases/plain_db', 'shard$(( (n + 1) / 2 ))', 'replica$(( (n + 1) % 2 + 1 ))')"; done
for n in 1 2 3 4; do query $n "CREATE DATABASE with_collection ENGINE = Replicated('/clickhouse/databases/with_collection', 'shard$(( (n + 1) / 2 ))', 'replica$(( (n + 1) % 2 + 1 ))') SETTINGS collection_name = 'cluster_credentials'"; done
result "the own cluster of a database without collection_name" "$(query 1 "SELECT count() FROM clusterAllReplicas('plain_db', system.one)")"
result "the own cluster of a database with collection_name" "$(query 1 "SELECT count() FROM clusterAllReplicas('with_collection', system.one)")"
result "a named cluster without credentials or a secret" "$(query 1 "SELECT count() FROM clusterAllReplicas('two_by_two', system.one)")"
result "a named cluster with a secret" "$(query 1 "SELECT count() FROM clusterAllReplicas('two_by_two_secret', system.one)")"
for n in 1 2 3 4; do query $n "CREATE USER reader IDENTIFIED WITH no_password"; query $n "GRANT SELECT ON *.*, REMOTE ON *.* TO reader"; done
result "currentUser() on every node for user reader, through the named cluster with a secret" "$(compose exec -T ch1 clickhouse-client --user reader -q "SELECT DISTINCT currentUser() FROM clusterAllReplicas('two_by_two_secret', system.one)" 2>&1 | cut -c1-200 | join_lines)"
result "a statement through the database without collection_name still reaches every node" "$(query 1 "CREATE TABLE plain_db.t (value Int32) ENGINE = MergeTree ORDER BY value" $DDL_OUTPUT_ONLY_ACTIVE | join_lines)"

test_id C2 "A changed default_replica_path and default_replica_name"
query 1 "CREATE TABLE with_collection.spans_local $SPAN_COLUMNS ENGINE = ReplicatedMergeTree ORDER BY trace_id" $DDL_OUTPUT_NONE; sleep 2
result "system.server_settings on ch1" "$(query 1 "SELECT name, value, changed FROM system.server_settings WHERE name IN ('default_replica_path', 'default_replica_name')" | join_lines)"
result "Keeper path, replica name and total_replicas of the table on every node" "$(per_node with_collection system.replicas "any(replaceRegexpOne(zookeeper_path, '[0-9a-f-]{36}', '<uuid>')), any(replica_name), any(total_replicas)" "database = 'with_collection'")"

test_id C3 "A cluster that ch4 defines with its shards in the other order"
result "distinct definitions of 'skewed' held by ch1, ch2 and ch4" "$(for n in 1 2 4; do query $n "SELECT groupArray((shard_num, host_name)) FROM (SELECT shard_num, host_name FROM system.clusters WHERE cluster = 'skewed' ORDER BY shard_num)"; done | sort -u | join_lines)"
for n in 1 2 4; do
  query $n "CREATE TABLE default.skewed_local (trace_id String) ENGINE = MergeTree ORDER BY trace_id"
  query $n "CREATE TABLE default.skewed AS default.skewed_local ENGINE = Distributed('skewed', currentDatabase(), 'skewed_local', cityHash64(trace_id))"
done
query 1 "INSERT INTO default.skewed SELECT toString(intDiv(number, 2)) FROM numbers(0, 1000) WHERE number % 2 = 0" $FOREGROUND_INSERT
query 4 "INSERT INTO default.skewed SELECT toString(intDiv(number, 2)) FROM numbers(0, 1000) WHERE number % 2 = 1" $FOREGROUND_INSERT
result "500 traces of two spans, one span inserted through ch1 and the other through ch4: traces with spans on both ch1 and ch2" "$(query 1 "SELECT count() FROM (SELECT trace_id FROM clusterAllReplicas('skewed', default.skewed_local) GROUP BY trace_id HAVING uniqExact(hostName()) > 1)")"

test_id C4 "A database that one node holds under another name"
for n in 1 2 3; do query $n "CREATE DATABASE named_db ENGINE = Replicated('/clickhouse/databases/named_db', 'shard$n', 'replica1')"; done
query 4 "CREATE DATABASE other_name ENGINE = Replicated('/clickhouse/databases/named_db', 'shard4', 'replica1')"
query 1 "CREATE TABLE spans_local (trace_id String) ENGINE = MergeTree ORDER BY trace_id" --database named_db $DDL_OUTPUT_NONE
query 1 "CREATE TABLE spans AS spans_local ENGINE = Distributed('four_by_one', currentDatabase(), 'spans_local', cityHash64(trace_id))" --database named_db $DDL_OUTPUT_NONE
sleep 3
result "databases that hold spans_local on ch4" "$(query 4 "SELECT database FROM system.tables WHERE name = 'spans_local' AND database IN ('named_db', 'other_name')" | join_lines)"
result "databases named named_db on ch1 to ch4" "$(for n in 1 2 3 4; do query $n "SELECT count() FROM system.databases WHERE name = 'named_db'"; done | join_lines)"

echo; echo "# Done"
