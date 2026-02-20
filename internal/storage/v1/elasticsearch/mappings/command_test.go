// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
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

func TestIsValidOption(t *testing.T) {
	tests := []struct {
		name          string
		arg           string
		expectedValue bool
	}{
		{name: "span mapping", arg: "jaeger-span", expectedValue: true},
		{name: "service mapping", arg: "jaeger-service", expectedValue: true},
		{name: "Invalid mapping", arg: "dependency-service", expectedValue: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MappingTypeFromString(test.arg)
			if test.expectedValue {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_getMappingAsString(t *testing.T) {
	tests := []struct {
		name    string
		args    Options
		want    string
		wantErr error
	}{
		{
			name: "ES version 7", args: Options{Mapping: "jaeger-span", EsVersion: 7, Shards: 5, Replicas: new(int64(1)), IndexPrefix: "test", UseILM: "true", ILMPolicyName: "jaeger-test-policy"},
			want: "ES version 7",
		},
		{
			name: "Parse Error version 7", args: Options{Mapping: "jaeger-span", EsVersion: 7, Shards: 5, Replicas: new(int64(1)), IndexPrefix: "test", UseILM: "true", ILMPolicyName: "jaeger-test-policy"},
			wantErr: errors.New("parse error"),
		},
		{
			name: "Parse bool error", args: Options{Mapping: "jaeger-span", EsVersion: 7, Shards: 5, Replicas: new(int64(1)), IndexPrefix: "test", UseILM: "foo", ILMPolicyName: "jaeger-test-policy"},
			wantErr: errors.New("strconv.ParseBool: parsing \"foo\": invalid syntax"),
		},
		{
			name: "Invalid Mapping type", args: Options{Mapping: "invalid-mapping", EsVersion: 7, Shards: 5, Replicas: new(int64(1)), IndexPrefix: "test", UseILM: "true", ILMPolicyName: "jaeger-test-policy"},
			wantErr: errors.New("invalid mapping type: invalid-mapping"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			mockTemplateApplier := &mocks.TemplateApplier{}
			mockTemplateApplier.On("Execute", mock.Anything, mock.Anything).Return(
				func(wr io.Writer, _ any) error {
					wr.Write([]byte(tt.want))
					return nil
				},
			)
			mockTemplateBuilder := &mocks.TemplateBuilder{}
			mockTemplateBuilder.On("Parse", mock.Anything).Return(mockTemplateApplier, tt.wantErr)

			// Test
			got, err := getMappingAsString(mockTemplateBuilder, tt.args)

			// Validate
			if tt.wantErr != nil {
				require.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
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
				Mapping: "jaeger-span",
				UseILM:  "foobar",
			},
			expectErr: "foobar",
		},
		{
			name: "valid jaeger-span mapping",
			options: Options{
				Mapping:       "jaeger-span",
				EsVersion:     7,
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
				Mapping:       "jaeger-service",
				EsVersion:     7,
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
			expectErr: "invalid mapping type ''",
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
