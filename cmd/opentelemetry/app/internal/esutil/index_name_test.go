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

package esutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndexNames(t *testing.T) {
	tests := []struct {
		name         string
		indices      []string
		nameProvider IndexNameProvider
		start        time.Time
		end          time.Time
	}{
		{
			name:         "index prefix",
			nameProvider: NewIndexNameProvider("myindex", "production", "2006-01-02", AliasNone, false),
			indices:      []string{"production-myindex-0001-01-01"},
		},
		{
			name:         "multiple dates",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasNone, false),
			indices:      []string{"myindex-2020-08-30", "myindex-2020-08-29", "myindex-2020-08-28"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use aliases",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasRead, false),
			indices:      []string{"myindex-read"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasNone, true),
			indices:      []string{"myindex-archive"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive alias",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasRead, true),
			indices:      []string{"myindex-archive-read"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indices := test.nameProvider.IndexNameRange(test.start, test.end)
			assert.Equal(t, test.indices, indices)
		})
	}
}

func TestIndexName(t *testing.T) {
	tests := []struct {
		name         string
		index        string
		nameProvider IndexNameProvider
		date         time.Time
		end          time.Time
	}{
		{
			name:         "index prefix",
			nameProvider: NewIndexNameProvider("myindex", "production", "2006-01-02", AliasNone, false),
			index:        "production-myindex-0001-01-01",
		},
		{
			name:         "no prefix",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasNone, false),
			index:        "myindex-2020-08-28",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use aliases",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasWrite, false),
			index:        "myindex-write",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasNone, true),
			index:        "myindex-archive",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive alias",
			nameProvider: NewIndexNameProvider("myindex", "", "2006-01-02", AliasWrite, true),
			index:        "myindex-archive-write",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indices := test.nameProvider.IndexName(test.date)
			assert.Equal(t, test.index, indices)
		})
	}
}
