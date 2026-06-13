// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expvar

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionConfig(t *testing.T) {
	config := createDefaultConfig().(*Config)
	err := config.Validate()
	require.NoError(t, err)
}
