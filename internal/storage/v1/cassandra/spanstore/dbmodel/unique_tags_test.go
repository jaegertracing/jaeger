// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
)

func TestGetUniqueTags(t *testing.T) {
	expectedTags := getTestUniqueTags()
	uniqueTags := GetAllUniqueTags(getTestJaegerSpan(), DefaultTagFilter)
	if !assert.EqualValues(t, expectedTags, uniqueTags) {
		for _, diff := range pretty.Diff(expectedTags, uniqueTags) {
			t.Log(diff)
		}
	}
}
