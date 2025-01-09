// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

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
	fcfg := EsRolloverFlagConfig{}
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	fcfg.AddFlagsForRolloverOptions(flags)
	fcfg.AddFlagsForLookBackOptions(flags)
	fcfg.AddFlagsForRollBackOptions(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags([]string{
		"--archive=true",
		"--timeout=150",
		"--es.ilm-policy-name=jaeger-ilm",
		"--skip-dependencies=true",
		"--adaptive-sampling=true",
		"--unit=days",
		"--unit-count=16",
		"--conditions={\"max_age\": \"20000d\"}",
	})
	require.NoError(t, err)

	c := fcfg.InitRolloverOptionsFromViper(v)
	l := fcfg.InitLookBackFromViper(v)
	r := fcfg.InitRollBackFromViper(v)
	assert.True(t, c.Archive)
	assert.Equal(t, 150, c.Timeout)
	assert.Equal(t, "jaeger-ilm", c.ILMPolicyName)
	assert.True(t, c.SkipDependencies)
	assert.True(t, c.AdaptiveSampling)
	assert.Equal(t, "days", l.Unit)
	assert.Equal(t, 16, l.UnitCount)
	assert.JSONEq(t, `{"max_age": "20000d"}`, r.Conditions)
}
