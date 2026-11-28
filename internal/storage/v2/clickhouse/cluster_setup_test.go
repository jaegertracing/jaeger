// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
)

func TestClusterSetupFailures(t *testing.T) {
	tests := []struct {
		name    string
		shards  []clusterShard
		wantErr []string
	}{
		{
			name:   "two shards, internal_replication on every shard",
			shards: []clusterShard{{ShardNum: 1, InternalReplication: 1}, {ShardNum: 2, InternalReplication: 1}},
		},
		{
			name: "two shards of two replicas, internal_replication on every shard",
			shards: []clusterShard{
				{ShardNum: 1, InternalReplication: 1},
				{ShardNum: 1, InternalReplication: 1},
				{ShardNum: 2, InternalReplication: 1},
				{ShardNum: 2, InternalReplication: 1},
			},
		},
		{
			name:    "cluster does not exist",
			shards:  nil,
			wantErr: []string{`cluster "my_cluster" does not exist in system.clusters`},
		},
		{
			name:    "one shard",
			shards:  []clusterShard{{ShardNum: 1, InternalReplication: 1}},
			wantErr: []string{`cluster "my_cluster" has one shard`},
		},
		{
			name:    "internal_replication off",
			shards:  []clusterShard{{ShardNum: 1, InternalReplication: 0}, {ShardNum: 2, InternalReplication: 0}},
			wantErr: []string{`does not set internal_replication on every shard`},
		},
		{
			name:    "internal_replication off on one shard",
			shards:  []clusterShard{{ShardNum: 1, InternalReplication: 1}, {ShardNum: 2, InternalReplication: 0}},
			wantErr: []string{`does not set internal_replication on every shard`},
		},
		{
			name:    "one shard and internal_replication off",
			shards:  []clusterShard{{ShardNum: 1, InternalReplication: 0}},
			wantErr: []string{`has one shard`, `does not set internal_replication`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := clusterSetupFailures("my_cluster", tt.shards)
			if len(tt.wantErr) == 0 {
				assert.Empty(t, failures)
				return
			}
			require.Len(t, failures, len(tt.wantErr))
			for i, want := range tt.wantErr {
				assert.Contains(t, failures[i], want)
			}
		})
	}
}

func TestCheckClusterSetup(t *testing.T) {
	validShards := []clusterShard{{ShardNum: 1, InternalReplication: 1}, {ShardNum: 2, InternalReplication: 1}}

	t.Run("no cluster configured", func(t *testing.T) {
		conn := &clickhousetest.Driver{}
		require.NoError(t, checkClusterSetup(t.Context(), conn, nil, false, zap.NewNop()))
		assert.Empty(t, conn.RecordedQueries)
	})

	t.Run("valid cluster passes", func(t *testing.T) {
		conn := clusterConn(t, validShards, nil)
		err := checkClusterSetup(t.Context(), conn, &ClusterConfig{Name: "my_cluster"}, false, zap.NewNop())
		require.NoError(t, err)
	})

	t.Run("invalid cluster stops startup by default", func(t *testing.T) {
		conn := clusterConn(t, []clusterShard{{ShardNum: 1, InternalReplication: 1}}, nil)
		err := checkClusterSetup(t.Context(), conn, &ClusterConfig{Name: "my_cluster"}, false, zap.NewNop())
		require.ErrorContains(t, err, "has one shard")
	})

	t.Run("invalid cluster only warns when warn is true", func(t *testing.T) {
		conn := clusterConn(t, []clusterShard{{ShardNum: 1, InternalReplication: 1}}, nil)
		core, observed := observer.New(zap.WarnLevel)
		err := checkClusterSetup(t.Context(), conn, &ClusterConfig{Name: "my_cluster"}, true, zap.New(core))
		require.NoError(t, err)
		require.Equal(t, 1, observed.Len())
		assert.Contains(t, observed.All()[0].Message, "has one shard")
	})

	t.Run("query failure is always an error, warn or not", func(t *testing.T) {
		conn := clusterConn(t, nil, assert.AnError)
		err := checkClusterSetup(t.Context(), conn, &ClusterConfig{Name: "my_cluster"}, true, zap.NewNop())
		require.ErrorContains(t, err, "failed to read cluster")
	})

	t.Run("row scan failure is always an error, warn or not", func(t *testing.T) {
		conn := &clickhousetest.Driver{
			QueryResponses: map[string]*clickhousetest.QueryResponse{
				"FROM system.clusters": {
					Rows: &clickhousetest.Rows[clusterShard]{
						Data:    validShards,
						ScanErr: assert.AnError,
					},
				},
			},
		}
		err := checkClusterSetup(t.Context(), conn, &ClusterConfig{Name: "my_cluster"}, true, zap.NewNop())
		require.ErrorContains(t, err, "failed to read cluster")
	})

	t.Run("row iteration failure is always an error, warn or not", func(t *testing.T) {
		conn := &clickhousetest.Driver{
			QueryResponses: map[string]*clickhousetest.QueryResponse{
				"FROM system.clusters": {
					Rows: &clickhousetest.Rows[clusterShard]{
						Data:    validShards,
						RowsErr: assert.AnError,
						ScanFn: func(dest any, src clusterShard) error {
							d := dest.([]any)
							*d[0].(*uint32) = src.ShardNum
							*d[1].(*uint8) = src.InternalReplication
							return nil
						},
					},
				},
			},
		}
		err := checkClusterSetup(t.Context(), conn, &ClusterConfig{Name: "my_cluster"}, true, zap.NewNop())
		require.ErrorContains(t, err, "failed to read cluster")
	})
}

// clusterConn returns a test double whose system.clusters query returns shards, or queryErr
// when it is set.
func clusterConn(t *testing.T, shards []clusterShard, queryErr error) *clickhousetest.Driver {
	t.Helper()
	return &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"FROM system.clusters": {
				Rows: &clickhousetest.Rows[clusterShard]{
					Data: shards,
					ScanFn: func(dest any, src clusterShard) error {
						d := dest.([]any)
						*d[0].(*uint32) = src.ShardNum
						*d[1].(*uint8) = src.InternalReplication
						return nil
					},
				},
				Err: queryErr,
			},
		},
	}
}
