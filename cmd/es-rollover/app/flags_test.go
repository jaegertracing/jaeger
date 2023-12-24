// Copyright (c) 2021 The Jaeger Authors.
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
}
