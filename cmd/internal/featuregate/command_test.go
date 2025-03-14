// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package featuregate

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCommand(t *testing.T) {
	cmd := Command()
	assert.Equal(t, "featuregate [feature-id]", cmd.Use)
}

func TestCommand_Panic(t *testing.T) {
	assert.PanicsWithValue(t, "could not find 'featuregate' command", func() {
		command(func() *cobra.Command {
			return &cobra.Command{}
		})
	})
}
