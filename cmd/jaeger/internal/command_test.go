// Copyright (c) 2024 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	require.ErrorContains(t, err, "bad-file-name")
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
}
