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

func TestCommand(t *testing.T) {
	cmd := Command()

	assert.NotNil(t, cmd)
	assert.Equal(t, "jaeger", cmd.Use)
	assert.Equal(t, description, cmd.Long)

	require.NotNil(t, cmd.RunE)

	cmd.ParseFlags([]string{"--config", "bad-file-name"})
	err := cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "bad-file-name")

	t.Run("Should have es-mappings command", func(t *testing.T) {
		foundCmd, _, _ := cmd.Find([]string{"es-mappings"})
		assert.NotNil(t, foundCmd, "es-mappings command should be present")
		assert.Equal(t, "es-mappings", foundCmd.Use, "Found command should match 'es-mappings'")
	})
}

func TestCheckConfigAndRun_DefaultConfig(t *testing.T) {
	cmd := &cobra.Command{
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			return nil
		},
	}
	cmd.Flags().String("config", "", "path to config file")
	getCfg := func( /* name */ string) ([]byte, error) {
		return []byte("default-config"), nil
	}
	runE := func(_ *cobra.Command, _ /* args */ []string,
	) error {
		return nil
	}

	err := checkConfigAndRun(cmd, nil, getCfg, runE)
	require.NoError(t, err)

	errGetCfg := errors.New("error")
	getCfgErr := func( /* name */ string) ([]byte, error) {
		return nil, errGetCfg
	}
	err = checkConfigAndRun(cmd, nil, getCfgErr, runE)
	require.ErrorIs(t, err, errGetCfg)

	t.Run("Should load custom config if provided", func(t *testing.T) {
		cmd.Flags().Set("config", "custom-config.yaml")
		getCfg := func(name string) ([]byte, error) {
			assert.Equal(t, "custom-config.yaml", name)
			return []byte("custom-config"), nil
		}

		err := checkConfigAndRun(cmd, nil, getCfg, runE)
		require.NoError(t, err)
	})
}
