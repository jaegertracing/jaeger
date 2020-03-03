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

package tlscfg

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientFlags(t *testing.T) {
	cmdLine := []string{
		"--prefix.tls.ca=ca-file",
		"--prefix.tls.cert=cert-file",
		"--prefix.tls.key=key-file",
		"--prefix.tls.server-name=HAL1",
		"--prefix.tls.skip-host-verify=true",
	}

	tests := []struct {
		option string
	}{
		{
			option: "--prefix.tls=true",
		},
		{
			option: "--prefix.tls.enabled=true",
		},
	}

	for _, test := range tests {
		t.Run(test.option, func(t *testing.T) {
			v := viper.New()
			command := cobra.Command{}
			flagSet := &flag.FlagSet{}
			flagCfg := ClientFlagsConfig{
				Prefix:         "prefix",
				Enabled:        Show,
				ShowServerName: true,
			}
			flagCfg.AddFlags(flagSet)
			command.PersistentFlags().AddGoFlagSet(flagSet)
			v.BindPFlags(command.PersistentFlags())

			err := command.ParseFlags(append(cmdLine, test.option))
			require.NoError(t, err)
			tlsOpts := flagCfg.InitFromViper(v)
			assert.Equal(t, Options{
				Enabled:        true,
				CAPath:         "ca-file",
				CertPath:       "cert-file",
				KeyPath:        "key-file",
				ServerName:     "HAL1",
				SkipHostVerify: true,
			}, tlsOpts)
		})
	}
}

func TestServerFlags(t *testing.T) {
	cmdLine := []string{
		"##placeholder##", // replaced in each test below
		"--prefix.tls=true",
		"--prefix.tls.cert=cert-file",
		"--prefix.tls.key=key-file",
	}

	tests := []struct {
		option string
		file   string
	}{
		{
			option: "--prefix.tls.client-ca=client-ca-file",
			file:   "client-ca-file",
		},
		{
			option: "--prefix.tls.client.ca=legacy-client-ca-file",
			file:   "legacy-client-ca-file",
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			v := viper.New()
			command := cobra.Command{}
			flagSet := &flag.FlagSet{}
			flagCfg := ServerFlagsConfig{
				Prefix:       "prefix",
				ShowEnabled:  Show,
				ShowClientCA: true,
			}
			flagCfg.AddFlags(flagSet)
			command.PersistentFlags().AddGoFlagSet(flagSet)
			v.BindPFlags(command.PersistentFlags())

			cmdLine[0] = test.option
			err := command.ParseFlags(cmdLine)
			require.NoError(t, err)
			tlsOpts := flagCfg.InitFromViper(v)
			assert.Equal(t, Options{
				Enabled:      true,
				CertPath:     "cert-file",
				KeyPath:      "key-file",
				ClientCAPath: test.file,
			}, tlsOpts)
		})
	}
}
