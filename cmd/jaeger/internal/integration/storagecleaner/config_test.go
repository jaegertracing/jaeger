// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorageExtensionConfig(t *testing.T) {
	config := createDefaultConfig().(*Config)
	config.TraceStorage = "storage"
	err := config.Validate()
	require.NoError(t, err)
}

func TestStorageExtensionConfigError(t *testing.T) {
	config := createDefaultConfig().(*Config)
	err := config.Validate()
	require.ErrorContains(t, err, "non zero value required")
}
