// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryapp "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal"
)

func Test_Validate(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedErr      string
		expectedBasePath string
	}{
		{
			name:        "Empty config",
			config:      &Config{},
			expectedErr: "Storage.TracesPrimary: non zero value required",
		},
		{
			name: "Non empty-config",
			config: &Config{
				Storage: Storage{
					TracesPrimary: "some-storage",
				},
			},
			expectedErr: "",
		},
		{
			name: "BasePath no leading slash",
			config: &Config{
				QueryOptions: queryapp.QueryOptions{BasePath: "no-leading-slash"},
				Storage:      Storage{TracesPrimary: "some-storage"},
			},
			expectedErr: "invalid base_path",
		},
		{
			name: "BasePath trailing slash normalized",
			config: &Config{
				QueryOptions: queryapp.QueryOptions{BasePath: "/jaeger/"},
				Storage:      Storage{TracesPrimary: "some-storage"},
			},
			expectedErr:      "",
			expectedBasePath: "/jaeger",
		},
		{
			name: "BasePath duplicate slashes rejected",
			config: &Config{
				QueryOptions: queryapp.QueryOptions{BasePath: "/jaeger//foo"},
				Storage:      Storage{TracesPrimary: "some-storage"},
			},
			expectedErr: "invalid base_path",
		},
		{
			name: "BasePath double leading slash rejected",
			config: &Config{
				QueryOptions: queryapp.QueryOptions{BasePath: "//jaeger"},
				Storage:      Storage{TracesPrimary: "some-storage"},
			},
			expectedErr: "invalid base_path",
		},
		{
			name: "BasePath path traversal rejected",
			config: &Config{
				QueryOptions: queryapp.QueryOptions{BasePath: "/jaeger/../other"},
				Storage:      Storage{TracesPrimary: "some-storage"},
			},
			expectedErr: "invalid base_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectedErr == "" {
				require.NoError(t, err)
				if tt.expectedBasePath != "" {
					assert.Equal(t, tt.expectedBasePath, tt.config.BasePath)
				}
			} else {
				assert.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}
