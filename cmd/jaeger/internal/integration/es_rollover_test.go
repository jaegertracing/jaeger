// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

func TestEsRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, "elasticsearch")
	e := integration.NewESRolloverIntegration(func(_ string, envs []string, adaptiveSampling bool) error {
		s := &E2EStorageIntegration{
			ConfigFile: "../../config-es-rollover.yaml",
			StorageIntegration: integration.StorageIntegration{
				CleanUp:  purge,
				Fixtures: integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			},
		}
		file, err := modifyConfigFileAsPerEnv(envs, adaptiveSampling)
		require.NoError(t, err)
		t.Cleanup(func() {
			// This is for over-all resetting of config if any of the test fails
			resetConfigFile(t, file)
		})
		s.e2eInitializeForCronJob(t, "elasticsearch", 1*time.Second)
		// This will reset the config everytime after the usage of rollover
		resetConfigFile(t, file)
		return nil
	}, false)
	e.RunESRolloverTests(t)
}

func modifyConfigFileAsPerEnv(envs []string, adaptiveSampling bool) ([]byte, error) {
	file, err := os.ReadFile("../../config-es-rollover.yaml")
	if err != nil {
		return nil, err
	}
	fileString := string(file)
	if adaptiveSampling {
		fileString = strings.Replace(fileString, "adaptive_sampling: false", "adaptive_sampling: true", 1)
	}
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		switch parts[0] {
		case "ES_USE_ILM":
			fileString = strings.Replace(fileString, "use_ilm: false", "use_ilm: true", 1)
		case "INDEX_PREFIX":
			ymlKey := "index_prefix"
			fileString = strings.Replace(fileString, getYamlKeyValue(ymlKey, "jaeger-main"), getYamlKeyValue(ymlKey, parts[1]), 1)
		case "ES_ILM_POLICY_NAME":
			ymlKey := "ilm_policy_name"
			fileString = strings.Replace(fileString, getYamlKeyValue(ymlKey, "jaeger-ilm-policy"), getYamlKeyValue(ymlKey, parts[1]), 1)
		}
	}
	err = os.WriteFile("../../config-es-rollover.yaml", []byte(fileString), 0o644)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func getYamlKeyValue(key, value string) string {
	return fmt.Sprintf("%s: \"%s\"", key, value)
}

func resetConfigFile(t *testing.T, file []byte) {
	err := os.WriteFile("../../config-es-rollover.yaml", file, 0o644)
	require.NoError(t, err)
}
