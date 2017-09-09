// Copyright (c) 2017 Uber Technologies, Inc.
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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestViperize(t *testing.T) {
	intFlag := "intFlag"
	stringFlag := "stringFlag"
	durationFlag := "durationFlag"

	expectedInt := 5
	expectedString := "string"
	expectedDuration := 13 * time.Second

	addFlags := func(flagSet *flag.FlagSet) {
		flagSet.Int(intFlag, 0, "")
		flagSet.String(stringFlag, "", "")
		flagSet.Duration(durationFlag, 0, "")
	}

	v, command := Viperize(addFlags)
	command.ParseFlags([]string{
		fmt.Sprintf("--%s=%d", intFlag, expectedInt),
		fmt.Sprintf("--%s=%s", stringFlag, expectedString),
		fmt.Sprintf("--%s=%s", durationFlag, expectedDuration.String()),
	})

	assert.Equal(t, expectedInt, v.GetInt(intFlag))
	assert.Equal(t, expectedString, v.GetString(stringFlag))
	assert.Equal(t, expectedDuration, v.GetDuration(durationFlag))
}

func TestEnv(t *testing.T) {
	envFlag := "jaeger.test-flag"
	actualEnvFlag := "JAEGER_TEST_FLAG"

	tempEnv := os.Getenv(actualEnvFlag)
	defer os.Setenv(actualEnvFlag, tempEnv)

	addFlags := func(flagSet *flag.FlagSet) {
		flagSet.String(envFlag, "", "")
	}
	expectedString := "string"
	os.Setenv(actualEnvFlag, expectedString)

	v, _ := Viperize(addFlags)
	assert.Equal(t, expectedString, v.GetString(envFlag))
}
