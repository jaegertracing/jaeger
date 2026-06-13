// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"
	"fmt"
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

	addFlags := func(flagSet *flag.FlagSet) {
		flagSet.String(envFlag, "", "")
	}
	expectedString := "string"
	t.Setenv(actualEnvFlag, expectedString)

	v, _ := Viperize(addFlags)
	assert.Equal(t, expectedString, v.GetString(envFlag))
}
