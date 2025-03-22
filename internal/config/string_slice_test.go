// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringSlice(t *testing.T) {
	f := &StringSlice{}

	assert.Equal(t, "[]", f.String())
	assert.Equal(t, "stringSlice", f.Type())

	f.Set("test")
	assert.Equal(t, `["test"]`, f.String())

	f.Set("test2")
	assert.Equal(t, `["test","test2"]`, f.String())

	f.Set("test3,test4")
	assert.Equal(t, `["test","test2","test3,test4"]`, f.String())
}

func TestStringSliceTreatedAsStringSlice(t *testing.T) {
	f := &StringSlice{}

	// create and add flags/values to a go flag set
	flagset := flag.NewFlagSet("test", flag.ContinueOnError)
	flagset.Var(f, "test", "test")

	err := flagset.Set("test", "asdf")
	require.NoError(t, err)
	err = flagset.Set("test", "blerg")
	require.NoError(t, err)
	err = flagset.Set("test", "other,thing")
	require.NoError(t, err)

	// add go flag set to pflag
	pflagset := pflag.FlagSet{}
	pflagset.AddGoFlagSet(flagset)
	actual, err := pflagset.GetStringSlice("test")
	require.NoError(t, err)

	assert.Equal(t, []string{"asdf", "blerg", "other,thing"}, actual)
}
