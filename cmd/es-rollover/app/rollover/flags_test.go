// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package rollover

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
		"--conditions={\"max_age\": \"20000d\"}",
	})
	require.NoError(t, err)

	c.InitFromViper(v)
	assert.JSONEq(t, `{"max_age": "20000d"}`, c.Conditions)
}
