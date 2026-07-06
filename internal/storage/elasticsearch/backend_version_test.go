// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSupportedVersion(t *testing.T) {
	for _, v := range AllVersions {
		assert.True(t, IsSupportedVersion(uint(v)), "%s should be supported", v)
	}
	for _, v := range []uint{0, 1, 5, 10, 100, 104} {
		assert.False(t, IsSupportedVersion(v), "%d should not be supported", v)
	}
}

func TestBackendVersion_String(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected string
	}{
		{ElasticV7, "Elasticsearch 7.x"},
		{ElasticV8, "Elasticsearch 8.x"},
		{ElasticV9, "Elasticsearch 9.x"},
		{OpenSearch1, "OpenSearch 1.x"},
		{OpenSearch2, "OpenSearch 2.x"},
		{OpenSearch3, "OpenSearch 3.x"},
		{BackendVersion(999), "Unknown(999)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.String())
	}
}

func TestBackendVersion_IsOpenSearch(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV7, false},
		{ElasticV8, false},
		{ElasticV9, false},
		{OpenSearch1, true},
		{OpenSearch2, true},
		{OpenSearch3, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.IsOpenSearch(), tt.version.String())
	}
}

func TestBackendVersion_TemplateVersion(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected uint
	}{
		{ElasticV7, 7},
		{ElasticV8, 8},
		{ElasticV9, 8},
		{OpenSearch1, 7},
		{OpenSearch2, 7},
		{OpenSearch3, 7},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.TemplateVersion(), tt.version.String())
	}
}

func TestBackendVersion_UsesV8API(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV7, false},
		{ElasticV8, true},
		{ElasticV9, true},
		{OpenSearch1, false},
		{OpenSearch2, false},
		{OpenSearch3, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.UsesV8API(), tt.version.String())
	}
}

func TestBackendVersion_SupportsILM(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV7, true},
		{ElasticV8, true},
		{ElasticV9, true},
		{OpenSearch1, true},
		{OpenSearch2, true},
		{OpenSearch3, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.SupportsILM(), tt.version.String())
	}
}

func TestDetectBackendVersion(t *testing.T) {
	tests := []struct {
		tagLine      string
		majorVersion int
		expected     BackendVersion
	}{
		{"You Know, for Search", 6, ElasticV8}, // Elasticsearch 6 unsupported → default
		{"You Know, for Search", 7, ElasticV7},
		{"You Know, for Search", 8, ElasticV8},
		{"You Know, for Search", 9, ElasticV9},
		{"You Know, for Search", 5, ElasticV8},
		{"The OpenSearch Project: https://opensearch.org/", 1, OpenSearch1},
		{"The OpenSearch Project: https://opensearch.org/", 2, OpenSearch2},
		{"The OpenSearch Project: https://opensearch.org/", 3, OpenSearch3},
		{"The OpenSearch Project: https://opensearch.org/", 4, OpenSearch3},
	}
	for _, tt := range tests {
		result := DetectBackendVersion(tt.tagLine, tt.majorVersion)
		assert.Equal(t, tt.expected, result, "tagLine=%q major=%d", tt.tagLine, tt.majorVersion)
	}
}

func TestResolveBackendVersion(t *testing.T) {
	errPing := errors.New("ping failed")
	tests := []struct {
		name        string
		configured  uint
		ping        func(context.Context) (PingResult, error)
		expected    BackendVersion
		errContains string
		wantPinged  bool
	}{
		{
			name:       "configured version skips ping",
			configured: uint(OpenSearch2),
			ping: func(context.Context) (PingResult, error) {
				return PingResult{}, errPing // must not be called
			},
			expected: OpenSearch2,
		},
		{
			name: "ping resolves version",
			ping: func(context.Context) (PingResult, error) {
				return PingResult{VersionNumber: "7.10.2", TagLine: "You Know, for Search"}, nil
			},
			expected:   ElasticV7,
			wantPinged: true,
		},
		{
			name: "ping error propagates",
			ping: func(context.Context) (PingResult, error) {
				return PingResult{}, errPing
			},
			errContains: "ping failed",
			wantPinged:  true,
		},
		{
			name: "empty version number",
			ping: func(context.Context) (PingResult, error) {
				return PingResult{VersionNumber: "", TagLine: "You Know, for Search"}, nil
			},
			errContains: "empty version number",
			wantPinged:  true,
		},
		{
			name: "non-numeric major version",
			ping: func(context.Context) (PingResult, error) {
				return PingResult{VersionNumber: "vNext", TagLine: "You Know, for Search"}, nil
			},
			errContains: "invalid version format: vNext",
			wantPinged:  true,
		},
		{
			// Regression: parsing only the first byte would read "10" as major 1
			// (OpenSearch1) instead of 10 (OpenSearch3).
			name: "multi-digit major is parsed fully",
			ping: func(context.Context) (PingResult, error) {
				return PingResult{VersionNumber: "10.0.0", TagLine: "The OpenSearch Project"}, nil
			},
			expected:   OpenSearch3,
			wantPinged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pinged bool
			ping := func(ctx context.Context) (PingResult, error) {
				pinged = true
				return tt.ping(ctx)
			}
			got, err := ResolveBackendVersion(context.Background(), tt.configured, ping)
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
			assert.Equal(t, tt.wantPinged, pinged)
		})
	}
}
