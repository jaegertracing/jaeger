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
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestCommand(t *testing.T) {
	expected := `-------------------------------------------------
| Configuration Option Name Value Source        |
-------------------------------------------------
| test_metrics              1     user-assigned |
| test_num_works            0     user-assigned |
| test_reporter_log_spans   true  user-assigned |
-------------------------------------------------
`
	v := viper.New()
	v.SetDefault("TEST_METRICS", "1")
	v.SetDefault("TEST_REPORTER_LOG_SPANS", "true")
	v.SetDefault("TEST_NUM_WORKS", "0")

	buf := new(bytes.Buffer)
	printCmd := Command(v)
	printCmd.SetOut(buf)
	_, err := printCmd.ExecuteC()
	if err != nil {
		require.NoError(t, err, "printCmd.ExecuteC() returned the error %v", err)
		actual := buf.String()
		assert.Equal(t, expected, actual)
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
