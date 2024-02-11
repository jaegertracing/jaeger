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
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestCommand(t *testing.T) {
	testPairs := []struct {
		Key   string
		Value string
	}{
		{"TEST_METRICS", "1"},
		{"TEST_REPORTER_LOG_SPANS", "true"},
		{"TEST_NUM_WORKS", "0"},
	}

	v := viper.New()

	v.SetDefault(testPairs[0].Key, testPairs[0].Value)
	v.SetDefault(testPairs[1].Key, testPairs[1].Value)
	v.SetDefault(testPairs[2].Key, testPairs[2].Value)

	buf := new(bytes.Buffer)
	printCmd := Command(v)
	printCmd.SetOut(buf)
	printCmd.ExecuteC()

	output := buf.String()

	for _, pair := range testPairs {

		key := strings.ToLower(pair.Key)
		value := strings.ToLower(pair.Value)
		str := fmt.Sprintf("%s=%s", key, value)

		assert.Contains(t, output, str, "Output should contain the value '%s' for key '%s'", value, key)
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
