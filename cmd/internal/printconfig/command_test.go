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
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllFlag(t *testing.T) {
	expected := `-------------------------------------------------------
| Configuration Option Name       Value Source        |
-------------------------------------------------------
| metrics_enabled_test                  user-assigned |
| new_feature_test                      default       |
| num_works_test                  5     user-assigned |
| reporter_log_spans_enabled_test true  default       |
| status.http.host_port_test      :8080 user-assigned |
-------------------------------------------------------
`

	v := viper.New()
	setConfig(v)
	actual := runPrintConfigCommand(v, t, true)
	assert.Equal(t, expected, actual)
}

func TestPrintConfigCommand(t *testing.T) {
	expected := `-------------------------------------------------------
| Configuration Option Name       Value Source        |
-------------------------------------------------------
| num_works_test                  5     user-assigned |
| reporter_log_spans_enabled_test true  default       |
| status.http.host_port_test      :8080 user-assigned |
-------------------------------------------------------
`
	v := viper.New()
	setConfig(v)
	actual := runPrintConfigCommand(v, t, false)
	assert.Equal(t, expected, actual)
}

func setConfig(v *viper.Viper) {

	v.Set("STATUS.HTTP.HOST_PORT_TEST", ":8080")
	v.Set("METRICS_ENABLED_TEST", "")
	v.Set("NEW_FEATURE_TEST", nil)

	if flag := pflag.Lookup("REPORTER_LOG_SPANS_ENABLED_TEST"); flag == nil {
		pflag.Bool("REPORTER_LOG_SPANS_ENABLED_TEST", true, "")
	}
	v.BindPFlags(pflag.CommandLine)

	os.Setenv("NUM_WORKS_TEST", "5")
	v.BindEnv("NUM_WORKS_TEST")
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
