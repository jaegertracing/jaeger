// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var _ TagFilter = &TagFilterDropAll{} // Check API compliance

func TestDropAll(t *testing.T) {
	sampleTags := model.KeyValues{
		model.String(someStringTagKey, someStringTagValue),
		model.Bool(someBoolTagKey, someBoolTagValue),
		model.Int64(someLongTagKey, someLongTagValue),
		model.Float64(someDoubleTagKey, someDoubleTagValue),
		model.Binary(someBinaryTagKey, someBinaryTagValue),
	}

	tt := []struct {
		filter              *TagFilterDropAll
		expectedTags        model.KeyValues
		expectedProcessTags model.KeyValues
		expectedLogs        model.KeyValues
	}{
		{
			filter:              NewTagFilterDropAll(false, false, false),
			expectedTags:        sampleTags,
			expectedProcessTags: sampleTags,
			expectedLogs:        sampleTags,
		},
		{
			filter:              NewTagFilterDropAll(true, false, false),
			expectedTags:        model.KeyValues{},
			expectedProcessTags: sampleTags,
			expectedLogs:        sampleTags,
		},
		{
			filter:              NewTagFilterDropAll(false, true, false),
			expectedTags:        sampleTags,
			expectedProcessTags: model.KeyValues{},
			expectedLogs:        sampleTags,
		},
		{
			filter:              NewTagFilterDropAll(false, false, true),
			expectedTags:        sampleTags,
			expectedProcessTags: sampleTags,
			expectedLogs:        model.KeyValues{},
		},
		{
			filter:              NewTagFilterDropAll(true, false, true),
			expectedTags:        model.KeyValues{},
			expectedProcessTags: sampleTags,
			expectedLogs:        model.KeyValues{},
		},
		{
			filter:              NewTagFilterDropAll(true, true, true),
			expectedTags:        model.KeyValues{},
			expectedProcessTags: model.KeyValues{},
			expectedLogs:        model.KeyValues{},
		},
	}

	for _, test := range tt {
		actualTags := test.filter.FilterTags(nil, sampleTags)
		assert.EqualValues(t, test.expectedTags, actualTags)

		actualProcessTags := test.filter.FilterProcessTags(nil, sampleTags)
		assert.EqualValues(t, test.expectedProcessTags, actualProcessTags)

		actualLogs := test.filter.FilterLogFields(nil, sampleTags)
		assert.EqualValues(t, test.expectedLogs, actualLogs)
	}
}
