// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"encoding/json"
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"github.com/crossdock/crossdock-go/require"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestGenerateMappings(t *testing.T) {
	tests := []struct {
		name      string
		options   app.Options
		expectErr bool
	}{
		{
			name: "valid jaeger-span mapping",
			options: app.Options{
				Mapping:       "jaeger-span",
				EsVersion:     7,
				Shards:        5,
				Replicas:      1,
				IndexPrefix:   "jaeger-index",
				UseILM:        "false",
				ILMPolicyName: "jaeger-ilm-policy",
			},
			expectErr: false,
		},
		{
			name: "valid jaeger-service mapping",
			options: app.Options{
				Mapping:       "jaeger-service",
				EsVersion:     7,
				Shards:        5,
				Replicas:      1,
				IndexPrefix:   "jaeger-service-index",
				UseILM:        "true",
				ILMPolicyName: "service-ilm-policy",
			},
			expectErr: false,
		},
		{
			name: "invalid mapping type",
			options: app.Options{
				Mapping: "invalid-mapping",
			},
			expectErr: true,
		},
		{
			name: "missing mapping flag",
			options: app.Options{
				Mapping: "",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateMappings(tt.options)
			if tt.expectErr {
				require.Error(t, err, "Expected an error")
			} else {
				require.NoError(t, err, "Did not expect an error")

				var parsed map[string]interface{}
				err = json.Unmarshal([]byte(result), &parsed)
				require.NoError(t, err, "Expected valid JSON output")

				assert.NotEmpty(t, parsed["index_patterns"], "Expected index_patterns to be present")
				assert.NotEmpty(t, parsed["mappings"], "Expected mappings to be present")
				assert.NotEmpty(t, parsed["settings"], "Expected settings to be present")
			}
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
