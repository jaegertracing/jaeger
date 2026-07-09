// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func TestOptionsWithDefaultFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}
	o.AddFlags(&c)

	assert.Empty(t, o.Mapping)
	// Version is resolved by PreRunE, not at flag-registration time.
	assert.Empty(t, c.Flags().Lookup(versionFlag).DefValue)
	assert.Equal(t, "7", c.Flags().Lookup(esVersionFlag).DefValue)
	assert.EqualValues(t, 5, o.Shards)
	assert.EqualValues(t, 1, *o.Replicas)

	assert.Empty(t, o.IndexPrefix)
	assert.Equal(t, "false", o.UseILM)
	assert.Equal(t, "jaeger-ilm-policy", o.ILMPolicyName)
}

func TestOptionsWithFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}

	o.AddFlags(&c)
	err := c.ParseFlags([]string{
		"--mapping=jaeger-span",
		"--es-version=7",
		"--shards=5",
		"--replicas=1",
		"--index-prefix=test",
		"--use-ilm=true",
		"--ilm-policy-name=jaeger-test-policy",
	})
	require.NoError(t, err)
	assert.Equal(t, "jaeger-span", o.Mapping)
	assert.Equal(t, int64(5), o.Shards)
	assert.Equal(t, int64(1), *o.Replicas)
	assert.Equal(t, "test", o.IndexPrefix)
	assert.Equal(t, "true", o.UseILM)
	assert.Equal(t, "jaeger-test-policy", o.ILMPolicyName)
}

func TestResolveBackendVersion(t *testing.T) {
	tests := []struct {
		name         string
		versionToken string
		legacy       uint
		expected     es.BackendVersion
		expectErr    string
	}{
		{
			name:     "legacy es-version used when token unset",
			legacy:   8,
			expected: es.ElasticV8,
		},
		{
			name:         "token selects opensearch",
			versionToken: "os3",
			expected:     es.OpenSearch3,
		},
		{
			name:         "token takes precedence over legacy es-version",
			versionToken: "os2",
			legacy:       8,
			expected:     es.OpenSearch2,
		},
		{
			name:         "invalid token surfaces error",
			versionToken: "os9",
			expectErr:    "invalid version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBackendVersion(tt.versionToken, tt.legacy)
			if tt.expectErr != "" {
				require.ErrorContains(t, err, tt.expectErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestVersionFlagResolution checks that AddFlags wires PreRunE to resolve the
// version flags into Options.Version when the command runs.
func TestVersionFlagResolution(t *testing.T) {
	newCmd := func() (*Options, *cobra.Command) {
		o := &Options{}
		c := &cobra.Command{RunE: func(*cobra.Command, []string) error { return nil }}
		o.AddFlags(c)
		return o, c
	}

	t.Run("token resolves and wins over legacy", func(t *testing.T) {
		o, c := newCmd()
		c.SetArgs([]string{"--mapping=jaeger-span", "--version=os2", "--es-version=8"})
		require.NoError(t, c.Execute())
		assert.Equal(t, es.OpenSearch2, o.Version)
	})

	t.Run("legacy es-version resolves when token unset", func(t *testing.T) {
		o, c := newCmd()
		c.SetArgs([]string{"--mapping=jaeger-span", "--es-version=9"})
		require.NoError(t, c.Execute())
		assert.Equal(t, es.ElasticV9, o.Version)
	})

	t.Run("invalid token fails the command", func(t *testing.T) {
		_, c := newCmd()
		c.SetArgs([]string{"--mapping=jaeger-span", "--version=os9"})
		require.ErrorContains(t, c.Execute(), "invalid version")
	})
}
