// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
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

package printconfig

import (
	"bytes"
	"flag"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/config/tlscfg"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	testPluginBinary             = "test-plugin.binary"
	testPluginConfigurationFile  = "test-plugin.configuration-file"
	testPluginLogLevel           = "test-plugin.log-level"
	testRemotePrefix             = "test-remote"
	testRemoteServer             = testRemotePrefix + ".server"
	testRemoteConnectionTimeout  = testRemotePrefix + ".connection-timeout"
	defaultTestPluginLogLevel    = "warn"
	defaultTestConnectionTimeout = time.Duration(5 * time.Second)
)

func addFlags(flagSet *flag.FlagSet) {
	tlscfg.ClientFlagsConfig{
		Prefix: "test",
	}.AddFlags(flagSet)

	flagSet.String(testPluginBinary, "", "")
	flagSet.String(testPluginConfigurationFile, "", "")
	flagSet.String(testPluginLogLevel, defaultTestPluginLogLevel, "")
	flagSet.String(testRemoteServer, "", "")
	flagSet.Duration(testRemoteConnectionTimeout, defaultTestConnectionTimeout, "")
}

func setConfig(t *testing.T) *viper.Viper {
	v, command := config.Viperize(addFlags, tenancy.AddFlags)
	err := command.ParseFlags([]string{
		"--test-plugin.binary=noop-test-plugin",
		"--test-plugin.configuration-file=config.json",
		"--test-plugin.log-level=debug",
		"--multi-tenancy.header=x-scope-orgid",
	})

	require.NoError(t, err)

	return v
}

func runPrintConfigCommand(v *viper.Viper, t *testing.T, allFlag bool) string {
	buf := new(bytes.Buffer)
	printCmd := Command(v)
	printCmd.SetOut(buf)

	if allFlag {
		err := printCmd.Flags().Set("all", "true")
		require.NoError(t, err, "printCmd.Flags() returned the error %v", err)
	}

	_, err := printCmd.ExecuteC()
	require.NoError(t, err, "printCmd.ExecuteC() returned the error %v", err)

	return buf.String()
}

func TestAllFlag(t *testing.T) {
	expected := `-----------------------------------------------------------------
| Configuration Option Name      Value            Source        |
-----------------------------------------------------------------
| multi-tenancy.enabled          false            default       |
| multi-tenancy.header           x-scope-orgid    user-assigned |
| multi-tenancy.tenants                           default       |
| test-plugin.binary             noop-test-plugin user-assigned |
| test-plugin.configuration-file config.json      user-assigned |
| test-plugin.log-level          debug            user-assigned |
| test-remote.connection-timeout 5s               default       |
| test-remote.server                              default       |
| test.tls.ca                                     default       |
| test.tls.cert                                   default       |
| test.tls.enabled               false            default       |
| test.tls.key                                    default       |
| test.tls.server-name                            default       |
| test.tls.skip-host-verify      false            default       |
-----------------------------------------------------------------
`

	v := setConfig(t)
	actual := runPrintConfigCommand(v, t, true)
	assert.Equal(t, expected, actual)
}

func TestPrintConfigCommand(t *testing.T) {
	expected := `-----------------------------------------------------------------
| Configuration Option Name      Value            Source        |
-----------------------------------------------------------------
| multi-tenancy.enabled          false            default       |
| multi-tenancy.header           x-scope-orgid    user-assigned |
| test-plugin.binary             noop-test-plugin user-assigned |
| test-plugin.configuration-file config.json      user-assigned |
| test-plugin.log-level          debug            user-assigned |
| test-remote.connection-timeout 5s               default       |
| test.tls.enabled               false            default       |
| test.tls.skip-host-verify      false            default       |
-----------------------------------------------------------------
`
	v := setConfig(t)
	actual := runPrintConfigCommand(v, t, false)
	assert.Equal(t, expected, actual)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
