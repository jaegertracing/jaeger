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
		"--index-date-separator=@",
		"--es.username=admin",
		"--es.password=admin",
	})
	require.NoError(t, err)

	require.NoError(t, c.InitFromViper(v))
	assert.Equal(t, "tenant1-", c.IndexPrefix)
	assert.True(t, c.Rollover)
	assert.True(t, c.Archive)
	assert.Equal(t, 150, c.MasterNodeTimeoutSeconds)
	assert.Equal(t, "@", c.IndexDateSeparator)
	assert.Equal(t, "admin", c.Username)
	assert.Equal(t, "admin", c.Password)
}

func TestInitFromViper_AuthFlagsBind(t *testing.T) {
	tests := []struct {
		name  string
		arg   string
		check func(t *testing.T, c *Config)
	}{
		{"token file", "--es.token-file=/etc/token", func(t *testing.T, c *Config) {
			assert.Equal(t, "/etc/token", c.TokenFilePath)
		}},
		{"api key file", "--es.api-key-file=/etc/apikey", func(t *testing.T, c *Config) {
			assert.Equal(t, "/etc/apikey", c.APIKeyFilePath)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v := viper.New()
			c := &Config{}
			command := cobra.Command{}
			flags := &flag.FlagSet{}
			c.AddFlags(flags)
			command.PersistentFlags().AddGoFlagSet(flags)
			v.BindPFlags(command.PersistentFlags())

			require.NoError(t, command.ParseFlags([]string{test.arg}))
			require.NoError(t, c.InitFromViper(v))
			test.check(t, c)
		})
	}
}

func TestInitFromViper_TLSError(t *testing.T) {
	v := viper.New()
	c := &Config{}
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	c.AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--es.tls.ca=/nonexistent/ca.crt",
	})
	require.NoError(t, err)

	err = c.InitFromViper(v)
	require.Error(t, err)
}
