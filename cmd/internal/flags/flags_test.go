// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJaegerTags(t *testing.T) {
	tags, err := ParseJaegerTags("")
	require.NoError(t, err)
	assert.Nil(t, tags)

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

	tags, err = ParseJaegerTags(jaegerTags)
	require.NoError(t, err)
	assert.Equal(t, expectedTags, tags)
}

func TestParseJaegerTagsError(t *testing.T) {
	_, err := ParseJaegerTags("no-equals-sign")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid Jaeger tag pair")
}
