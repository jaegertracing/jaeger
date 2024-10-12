// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

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
		"--shards=8",
		"--replicas=16",
		"--priority-span-template=300",
		"--priority-service-template=301",
		"--priority-dependencies-template=302",
		"--priority-sampling-template=303",
	})
	require.NoError(t, err)

	c.InitFromViper(v)
	assert.EqualValues(t, 8, c.Indices.Spans.Shards)
	assert.EqualValues(t, 16, c.Indices.Spans.Replicas)
	assert.EqualValues(t, 300, c.Indices.Spans.Priority)
	assert.EqualValues(t, 301, c.Indices.Services.Priority)
	assert.EqualValues(t, 302, c.Indices.Dependencies.Priority)
	assert.EqualValues(t, 303, c.Indices.Sampling.Priority)
}
