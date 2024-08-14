// Copyright (c) 2021 The Jaeger Authors.
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

package lookback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetTimeReference(t *testing.T) {
	now := time.Date(2021, time.October, 10, 10, 10, 10, 10, time.UTC)

	tests := []struct {
		name         string
		unit         string
		unitCount    int
		expectedTime time.Time
	}{
		{
			name:         "seconds unit",
			unit:         "seconds",
			unitCount:    30,
			expectedTime: time.Date(2021, time.October, 10, 10, 9, 40, 0o0, time.UTC),
		},
		{
			name:         "minutes unit",
			unit:         "minutes",
			unitCount:    30,
			expectedTime: time.Date(2021, time.October, 10, 9, 40, 0o0, 0o0, time.UTC),
		},
		{
			name:         "hours unit",
			unit:         "hours",
			unitCount:    2,
			expectedTime: time.Date(2021, time.October, 10, 8, 0o0, 0o0, 0o0, time.UTC),
		},
		{
			name:         "days unit",
			unit:         "days",
			unitCount:    2,
			expectedTime: time.Date(2021, 10, 9, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "weeks unit",
			unit:         "weeks",
			unitCount:    2,
			expectedTime: time.Date(2021, time.September, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "months unit",
			unit:         "months",
			unitCount:    2,
			expectedTime: time.Date(2021, time.August, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "years unit",
			unit:         "years",
			unitCount:    2,
			expectedTime: time.Date(2019, time.October, 10, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := getTimeReference(now, test.unit, test.unitCount)
			assert.Equal(t, test.expectedTime, ref)
		})
	}
}
