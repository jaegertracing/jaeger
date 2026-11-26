// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

// checkClusterSetup validates the named cluster against RFC 0022 §6.4 and §6.7: that it
// exists in system.clusters, has more than one shard, and sets internal_replication on
// every shard. A cluster with only one shard adds nothing over table_engine.replicated
// alone, and a cluster that does not set internal_replication makes a Distributed table
// write every insert to every replica of a shard in addition to the copy that replication
// already makes, storing each row more than once (RFC 0022 §6.7, tests S5 and R11).
//
// This covers only what the connected node's own system.clusters can show. It does not
// check that every node agrees on the cluster's definition, that the named cluster groups
// the nodes as their shard macros do, or that it lists the same nodes as the database's own
// cluster; those checks are left for a follow-up, as RFC 0022 §6.4 lists them separately.
//
// A failed check stops startup, unless warn is true, in which case it is logged instead.
func checkClusterSetup(ctx context.Context, conn driver.Conn, cluster *ClusterConfig, warn bool, logger *zap.Logger) error {
	if cluster == nil {
		return nil
	}
	shards, err := readClusterShards(ctx, conn, cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to read cluster %q from system.clusters: %w", cluster.Name, err)
	}
	failures := clusterSetupFailures(cluster.Name, shards)
	if len(failures) == 0 {
		return nil
	}
	msg := strings.Join(failures, "; ")
	if warn {
		logger.Warn("cluster setup validation failed: " + msg)
		return nil
	}
	return errors.New(msg)
}

// clusterShard is one (shard, replica) row of system.clusters.
type clusterShard struct {
	ShardNum            uint32
	InternalReplication uint8
}

// clusterSetupFailures returns the RFC 0022 §6.4/§6.7 violations the given shards show for
// the named cluster, or nil when none. An empty shards slice means the cluster does not
// exist on the connected node.
func clusterSetupFailures(name string, shards []clusterShard) []string {
	if len(shards) == 0 {
		return []string{fmt.Sprintf("cluster %q does not exist in system.clusters", name)}
	}
	var failures []string
	shardNums := map[uint32]struct{}{}
	internalReplicationOff := false
	for _, s := range shards {
		shardNums[s.ShardNum] = struct{}{}
		if s.InternalReplication == 0 {
			internalReplicationOff = true
		}
	}
	if len(shardNums) <= 1 {
		failures = append(failures, fmt.Sprintf(
			"cluster %q has one shard; a cluster block over one shard adds nothing, "+
				"remove table_engine.cluster and use table_engine.replicated alone (RFC 0022 §6.6)", name))
	}
	if internalReplicationOff {
		failures = append(failures, fmt.Sprintf(
			"cluster %q does not set internal_replication on every shard; without it a Distributed table "+
				"writes every insert to every replica of a shard in addition to the copy that replication "+
				"already makes, so each replica stores every row more than once (RFC 0022 §6.7)", name))
	}
	return failures
}

// readClusterShards reads the rows to the end, because the driver reports an error of the
// result stream only after the last row.
func readClusterShards(ctx context.Context, conn driver.Conn, name string) ([]clusterShard, error) {
	rows, err := conn.Query(ctx, sql.SelectClusterShards, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shards []clusterShard
	for rows.Next() {
		var s clusterShard
		if err := rows.Scan(&s.ShardNum, &s.InternalReplication); err != nil {
			return nil, err
		}
		shards = append(shards, s)
	}
	return shards, rows.Err()
}
