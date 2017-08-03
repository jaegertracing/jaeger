// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
