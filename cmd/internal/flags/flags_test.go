// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJaegerTags(t *testing.T) {
	assert.Nil(t, ParseJaegerTags(""))

	jaegerTags := fmt.Sprintf("%s,%s,%s,%s,%s,%s",
		"key=value",
		"envVar1=${envKey1:defaultVal1}",
		"envVar2=${envKey2:defaultVal2}",
		"envVar3=${envKey3}",
		"envVar4=${envKey4}",
		"envVar5=${envVar5:}",
	)

	t.Setenv("envKey1", "envVal1")
	t.Setenv("envKey4", "envVal4")

	expectedTags := map[string]string{
		"key":     "value",
		"envVar1": "envVal1",
		"envVar2": "defaultVal2",
		"envVar4": "envVal4",
		"envVar5": "",
	}

	assert.Equal(t, expectedTags, ParseJaegerTags(jaegerTags))
}

func TestParseJaegerTagsPanic(t *testing.T) {
	assert.PanicsWithValue(t,
		"invalid Jaeger tag pair \"no-equals-sign\", expected key=value",
		func() {
			ParseJaegerTags("no-equals-sign")
		},
	)
}
