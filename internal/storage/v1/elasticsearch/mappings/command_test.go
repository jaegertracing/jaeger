// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestCommandExecute(t *testing.T) {
	cmd := Command()

	// TempFile to capture output
	tempFile, err := os.Create(t.TempDir() + "command-output-*.txt")
	require.NoError(t, err)

	// Redirect stdout to the TempFile
	oldStdout := os.Stdout
	os.Stdout = tempFile
	defer func() { os.Stdout = oldStdout }()

	err = cmd.ParseFlags([]string{
		"--mapping=jaeger-span",
		"--es-version=7",
		"--shards=5",
		"--replicas=1",
		"--index-prefix=jaeger-index",
		"--use-ilm=false",
		"--ilm-policy-name=jaeger-ilm-policy",
	})
	require.NoError(t, err)
	require.NoError(t, cmd.Execute())

	output, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)

	var jsonOutput map[string]any
	err = json.Unmarshal(output, &jsonOutput)
	require.NoError(t, err, "Output should be valid JSON")
}

func TestCommandExecuteError(t *testing.T) {
	cmd := Command()
	require.NoError(t, cmd.ParseFlags([]string{"--mapping=foobar"}))
	require.ErrorContains(t, cmd.Execute(), "foobar")
}

func TestGenerateMappings(t *testing.T) {
	tests := []struct {
		name      string
		options   Options
		expectErr string
	}{
		{
			name: "bad ILM setting",
			options: Options{
				Mapping: config.SpanIndexName,
				UseILM:  "foobar",
			},
			expectErr: "foobar",
		},
		{
			name: "render error surfaced",
			options: Options{
				Mapping: config.SpanIndexName,
				UseILM:  "false",
				// no Replicas → RenderIndexTemplate fails, and generateMappings wraps it.
			},
			expectErr: "failed to render mapping",
		},
		{
			name: "valid jaeger-span mapping",
			options: Options{
				Mapping:       config.SpanIndexName,
				Version:       es.ElasticV7,
				Shards:        5,
				Replicas:      new(int64(1)),
				IndexPrefix:   "jaeger-index",
				UseILM:        "false",
				ILMPolicyName: "jaeger-ilm-policy",
			},
			expectErr: "",
		},
		{
			name: "valid jaeger-service mapping",
			options: Options{
				Mapping:       config.ServiceIndexName,
				Version:       es.ElasticV7,
				Shards:        5,
				Replicas:      new(int64(1)),
				IndexPrefix:   "jaeger-service-index",
				UseILM:        "true",
				ILMPolicyName: "service-ilm-policy",
			},
			expectErr: "",
		},
		{
			name: "invalid mapping type",
			options: Options{
				Mapping: "invalid-mapping",
			},
			expectErr: "invalid-mapping",
		},
		{
			name: "missing mapping flag",
			options: Options{
				Mapping: "",
			},
			expectErr: `invalid mapping type ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateMappings(tt.options)
			if tt.expectErr != "" {
				require.ErrorContains(t, err, tt.expectErr)
			} else {
				require.NoError(t, err, "Did not expect an error")

				var parsed map[string]any
				err = json.Unmarshal([]byte(result), &parsed)
				require.NoError(t, err, "Expected valid JSON output")

				assert.NotEmpty(t, parsed["index_patterns"], "Expected index_patterns to be present")
				assert.NotEmpty(t, parsed["mappings"], "Expected mappings to be present")
				assert.NotEmpty(t, parsed["settings"], "Expected settings to be present")
			}
		})
	}
}

func TestGenerateMappingsOpenSearchISM(t *testing.T) {
	result, err := generateMappings(Options{
		Mapping:       config.SpanIndexName,
		Version:       es.OpenSearch3,
		Shards:        5,
		Replicas:      new(int64(1)),
		UseILM:        "true",
		ILMPolicyName: "jaeger-ilm-policy",
	})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &parsed))

	settings, ok := parsed["settings"].(map[string]any)
	require.True(t, ok, "settings block should be present")
	// OpenSearch renders the ISM rollover alias; Elasticsearch would emit a
	// "lifecycle" block instead.
	assert.Contains(t, settings, "plugins.index_state_management.rollover_alias")
	assert.NotContains(t, settings, "lifecycle")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
