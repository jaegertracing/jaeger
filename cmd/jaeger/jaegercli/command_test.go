// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegercli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

func TestNewCommand(t *testing.T) {
	factories, err := internal.Components()
	require.NoError(t, err)

	cmd := NewCommand(factories)

	assert.Equal(t, "jaeger", cmd.Use)
	assert.Equal(t, description, cmd.Long)
	assert.Equal(t, description, cmd.Short)
	require.NotNil(t, cmd.RunE)

	cmd.ParseFlags([]string{"--config", "bad-file-name"})
	err = cmd.Execute()
	require.ErrorContains(t, err, "bad-file-name")
}

func TestCheckConfigAndRun(t *testing.T) {
	cmd := &cobra.Command{
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			return nil
		},
	}
	cmd.Flags().String("config", "", "path to config file")

	getCfg := func( /* name */ string) ([]byte, error) {
		return []byte("default-config"), nil
	}
	runE := func(_ *cobra.Command, _ /* args */ []string) error {
		return nil
	}

	err := checkConfigAndRun(cmd, nil, getCfg, runE)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(cmd.Flag("config").Value.String(), "yaml:"), "expected config flag to be set to yaml: payload")

	errGetCfg := errors.New("error")
	getCfgErr := func( /* name */ string) ([]byte, error) {
		return nil, errGetCfg
	}
	err = checkConfigAndRun(cmd, nil, getCfgErr, runE)
	require.ErrorIs(t, err, errGetCfg)
}

func TestAllInOneYAML_Readable(t *testing.T) {
	data, err := allInOneYAML.ReadFile("all-in-one.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestNewCommand_Subcommands(t *testing.T) {
	factories, err := internal.Components()
	require.NoError(t, err)

	cmd := NewCommand(factories)

	subcommands := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	assert.True(t, subcommands["version"], "expected version subcommand")
	assert.True(t, subcommands["docs"], "expected docs subcommand")
	assert.True(t, subcommands["elasticsearch-mappings"], "expected elasticsearch-mappings subcommand")
}
