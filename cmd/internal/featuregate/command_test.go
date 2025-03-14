// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package featuregate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddCommand(t *testing.T) {
	cmd := Command()
	assert.Equal(t, "featuregate [feature-id]", cmd.Use)
}
