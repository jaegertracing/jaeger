// Copyright (c) 2020 The Jaeger Authors.
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

package esspanreader

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestValidateQuery(t *testing.T) {
	tests := []struct {
		query *spanstore.TraceQueryParameters
		err   error
	}{
		{
			err: errNilQuery,
		},
		{
			query: &spanstore.TraceQueryParameters{Tags: map[string]string{"foo": "bar"}},
			err:   errServiceNameNotSet,
		},
		{
			query: &spanstore.TraceQueryParameters{},
			err:   errStartAndEndTimeNotSet,
		},
		{
			query: &spanstore.TraceQueryParameters{StartTimeMax: time.Now().Add(-time.Hour), StartTimeMin: time.Now()},
			err:   errStartTimeMinGreaterThanMax,
		},
		{
			query: &spanstore.TraceQueryParameters{
				StartTimeMax: time.Now(), StartTimeMin: time.Now().Add(-time.Hour),
				DurationMin: time.Hour, DurationMax: time.Minute,
			},
			err: errDurationMinGreaterThanMax,
		},
		{
			query: &spanstore.TraceQueryParameters{
				StartTimeMax: time.Now(), StartTimeMin: time.Now().Add(-time.Hour),
				DurationMin: time.Minute, DurationMax: time.Hour,
			},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test.err), func(t *testing.T) {
			err := validateQuery(test.query)
			if test.err != nil {
				assert.EqualError(t, err, test.err.Error())
				return
			}
			assert.NoError(t, err)
		})
	}
}
