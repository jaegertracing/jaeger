// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	factories, err := Components()
	require.NoError(t, err)

	cmd := NewCommand(factories)

	assert.NotNil(t, cmd)
	assert.Equal(t, "jaeger", cmd.Use)
	assert.Equal(t, description, cmd.Long)
	require.NotNil(t, cmd.RunE)

	subcommands := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	assert.True(t, subcommands["version"], "expected version subcommand")
	assert.True(t, subcommands["docs"], "expected docs subcommand")
	assert.True(t, subcommands["elasticsearch-mappings"], "expected elasticsearch-mappings subcommand")

	cmd.SetArgs([]string{"--config", "bad-file-name"})
	err = cmd.Execute()
	require.ErrorContains(t, err, "bad-file-name")
}

func TestCheckConfigAndRun_DefaultConfig(t *testing.T) {
	cmd := &cobra.Command{
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	cmd.Flags().String("config", "", "path to config file")
	getCfg := func(string) ([]byte, error) {
		return []byte("default-config"), nil
	}
	runE := func(_ *cobra.Command, _ []string) error {
		return nil
	}

	err := checkConfigAndRun(cmd, nil, getCfg, runE)
	require.NoError(t, err)

	errGetCfg := errors.New("error")
	getCfgErr := func(string) ([]byte, error) {
		return nil, errGetCfg
	}
	err = checkConfigAndRun(cmd, nil, getCfgErr, runE)
	require.ErrorIs(t, err, errGetCfg)
}
