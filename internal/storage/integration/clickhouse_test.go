// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/featuregate"

	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type ClickHouseStorageIntegration struct {
	StorageIntegration
	factory *ch.Factory
}

func (s *ClickHouseStorageIntegration) initialize(t *testing.T) {
	require.NoError(t, featuregate.GlobalRegistry().Set("storage.clickhouse", true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set("storage.clickhouse", false))
	})

	cfg := ch.Configuration{
		Addresses: []string{"127.0.0.1:9000"},
		Database:  "jaeger",
		Auth: ch.Authentication{
			Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
				Username: "default",
				Password: "password",
			}),
		},
	}
	f, err := ch.NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	s.factory = f

	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
}

func (s *ClickHouseStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func TestClickHouseStorage(t *testing.T) {
	SkipUnlessEnv(t, "clickhouse")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &ClickHouseStorageIntegration{
		StorageIntegration: StorageIntegration{
			SkipList: []string{"GetThroughput", "GetLatestProbability"},
		},
	}
	s.CleanUp = s.cleanUp
	s.initialize(t)
	s.RunAll(t)
}
