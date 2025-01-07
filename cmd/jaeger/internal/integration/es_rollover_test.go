// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestEsRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")
	e := integration.NewESRolloverIntegration(func(action string, envs []string, adaptiveSampling bool) error {
		s := &E2EStorageIntegration{
			ConfigFile: "../../config-es-rollover.yaml",
			StorageIntegration: integration.StorageIntegration{
				CleanUp:  purge,
				Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			},
			BinaryName: "jaeger-v2-collector",
		}
		envOverRides := make(map[string]string)
		for _, env := range envs {
			parts := strings.SplitN(env, "=", 2)
			var key string
			key = "ES_"
			if parts[0] != "ES_USE_ILM" && parts[0] != "INDEX_PREFIX" {
				key = key + "AUTO_ROLLOVER_" + parts[0]
			} else {
				if parts[0] == "ES_USE_ILM" {
					key = key + "USE_ILM"
				} else {
					key = key + parts[0]
				}
			}
			envOverRides[key] = parts[1]
		}
		s.EnvVarOverrides = envOverRides
		s.e2eInitializeForCronJob(t, "elasticsearch", 30*time.Second)
		return nil
	})
	e.RunESRolloverTests(t, false)
}
