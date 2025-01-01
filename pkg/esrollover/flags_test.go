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
	AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--index-prefix=tenant1",
		"--archive=true",
		"--timeout=150",
		"--es.username=admin",
		"--es.password=qwerty123",
		"--es.use-ilm=true",
		"--es.ilm-policy-name=jaeger-ilm",
		"--skip-dependencies=true",
		"--adaptive-sampling=true",
	})
	require.NoError(t, err)

	c.InitFromViper(v)
	assert.Equal(t, "tenant1-", c.IndexPrefix)
	assert.True(t, c.Archive)
	assert.Equal(t, 150, c.Timeout)
	assert.Equal(t, "admin", c.Username)
	assert.Equal(t, "qwerty123", c.Password)
	assert.Equal(t, "jaeger-ilm", c.ILMPolicyName)
	assert.True(t, c.SkipDependencies)
	assert.True(t, c.AdaptiveSampling)
}
