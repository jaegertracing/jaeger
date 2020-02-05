// Copyright (c) 2020 The Jaeger Authors.
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

package config

import (
	"flag"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	err = flagset.Set("test", "blerg")
	assert.NoError(t, err)
	err = flagset.Set("test", "other,thing")
	assert.NoError(t, err)

	// add go flag set to pflag
	pflagset := pflag.FlagSet{}
	pflagset.AddGoFlagSet(flagset)
	actual, err := pflagset.GetStringSlice("test")
	assert.NoError(t, err)

	assert.Equal(t, []string{"asdf", "blerg", "other,thing"}, actual)
}
