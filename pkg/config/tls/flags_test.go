// Copyright (c) 2019 The Jaeger Authors.
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

package tls

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlags(t *testing.T) {
	cmdLine := []string{
		"--prefix.tls=true",
		"--prefix.tls.ca=ca-file",
		"--prefix.tls.cert=cert-file",
		"--prefix.tls.key=key-file",
		"--prefix.tls.server-name=HAL1",
	}

	v := viper.New()
	command := cobra.Command{}
	flagSet := &flag.FlagSet{}
	flagCfg := FlagsConfig{
		Prefix:         "prefix.",
		ShowEnabled:    true,
		ShowServerName: true,
	}
	flagCfg.AddFlags(flagSet)
	command.PersistentFlags().AddGoFlagSet(flagSet)
	v.BindPFlags(command.PersistentFlags())

	err := command.ParseFlags(cmdLine)
	require.NoError(t, err)
	tlsOpts := flagCfg.InitFromViper(v)
	assert.Equal(t, Options{
		Enabled:    true,
		CAPath:     "ca-file",
		CertPath:   "cert-file",
		KeyPath:    "key-file",
		ServerName: "HAL1",
	}, tlsOpts)
}
