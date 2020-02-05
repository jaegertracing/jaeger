package config

import (
	"flag"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestStringSlice(t *testing.T) {
	f := &StringSlice{}

	assert.Equal(t, f.String(), "[]")
	assert.Equal(t, f.Type(), "stringSlice")

	f.Set("test")
	assert.Equal(t, f.String(), "[test]")

	f.Set("test2")
	assert.Equal(t, f.String(), "[test,test2]")
}

func TestStringSliceTreatedAsStringSlice(t *testing.T) {
	f := &StringSlice{}

	// create and add flags/values to a go flag set
	flagset := flag.NewFlagSet("test", flag.ContinueOnError)
	flagset.Var(f, "test", "test")

	err := flagset.Set("test", "asdf")
	assert.NoError(t, err)
	err = flagset.Set("test", "blerg")
	assert.NoError(t, err)

	// add go flag set to pflag
	pflagset := pflag.FlagSet{}
	pflagset.AddGoFlagSet(flagset)
	actual, err := pflagset.GetStringSlice("test")
	assert.NoError(t, err)

	assert.Equal(t, []string{"asdf", "blerg"}, actual)
}
