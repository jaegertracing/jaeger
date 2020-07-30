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

package spanreader

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndexName(t *testing.T) {
	tests := []struct {
		name         string
		indices      []string
		nameProvider indexNameProvider
		start        time.Time
		end          time.Time
	}{
		{
			name:         "index prefix",
			nameProvider: newIndexNameProvider("myindex", "production", false, false),
			indices:      []string{"production-myindex-0001-01-01"},
		},
		{
			name:         "multiple dates",
			nameProvider: newIndexNameProvider("myindex", "", false, false),
			indices:      []string{"myindex-2020-08-30", "myindex-2020-08-29", "myindex-2020-08-28"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use aliases",
			nameProvider: newIndexNameProvider("myindex", "", true, false),
			indices:      []string{"myindex-read"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive",
			nameProvider: newIndexNameProvider("myindex", "", false, true),
			indices:      []string{"myindex-archive"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive alias",
			nameProvider: newIndexNameProvider("myindex", "", true, true),
			indices:      []string{"myindex-archive-read"},
			start:        time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
			end:          time.Date(2020, 8, 30, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indices := test.nameProvider.get(test.start, test.end)
			assert.Equal(t, test.indices, indices)
		})
	}
}
