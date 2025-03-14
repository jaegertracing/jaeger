// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package featuregate

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAddCommand(t *testing.T) {
	testingCommand := &cobra.Command{
		Use: "testing",
	}
	AddCommand(testingCommand)
	assert.Len(t, testingCommand.Commands(), 1)
	assert.Equal(t, "featuregate [feature-id]", testingCommand.Commands()[0].Use)
}
