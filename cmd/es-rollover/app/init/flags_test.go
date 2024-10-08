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
	assert.Equal(t, int64(8), c.Indices.Spans.Shards)
	assert.Equal(t, int64(16), c.Indices.Spans.Replicas)
	assert.Equal(t, int64(300), c.Indices.Spans.Priority)
	assert.Equal(t, int64(301), c.Indices.Services.Priority)
	assert.Equal(t, int64(302), c.Indices.Dependencies.Priority)
	assert.Equal(t, int64(303), c.Indices.Sampling.Priority)
}
