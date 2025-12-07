// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_DoesNotReturnErrorWhenValid(t *testing.T) {
	tests := []struct {
		name   string
		config *Configuration
	}{
		{
			name:   "non-required fields not set",
			config: &Configuration{},
		},
		{
			name: "all fields are set",
			config: &Configuration{
				MaxTraces: 100,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			require.NoError(t, err)
		})
	}
}
