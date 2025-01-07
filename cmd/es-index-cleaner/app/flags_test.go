// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBindFlags(t *testing.T) {
	v := viper.New()
	c := &Config{}
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	c.AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--index-prefix=tenant1",
		"--rollover=true",
		"--archive=true",
		"--timeout=150",
		"--ignore-unavailable=false",
		"--index-date-separator=@",
		"--es.username=admin",
		"--es.password=admin",
	})
	require.NoError(t, err)

	c.InitFromViper(v)
	assert.Equal(t, "tenant1-", c.IndexPrefix)
	assert.True(t, c.Rollover)
	assert.True(t, c.Archive)
	assert.Equal(t, 150, c.MasterNodeTimeoutSeconds)
	assert.False(t, c.IgnoreUnavailableIndex)
	assert.Equal(t, "@", c.IndexDateSeparator)
	assert.Equal(t, "admin", c.Username)
	assert.Equal(t, "admin", c.Password)
}
