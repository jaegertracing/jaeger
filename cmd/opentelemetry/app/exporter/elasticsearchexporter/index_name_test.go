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

package elasticsearchexporter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIndexName(t *testing.T) {
	tests := []struct {
		name         string
		index        string
		nameProvider indexNameProvider
		date         time.Time
		end          time.Time
	}{
		{
			name:         "index prefix",
			nameProvider: newIndexNameProvider("myindex", "production", false, false),
			index:        "production-myindex-0001-01-01",
		},
		{
			name:         "no prefix",
			nameProvider: newIndexNameProvider("myindex", "", false, false),
			index:        "myindex-2020-08-28",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use aliases",
			nameProvider: newIndexNameProvider("myindex", "", true, false),
			index:        "myindex-write",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive",
			nameProvider: newIndexNameProvider("myindex", "", false, true),
			index:        "myindex-archive",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "use archive alias",
			nameProvider: newIndexNameProvider("myindex", "", true, true),
			index:        "myindex-archive-write",
			date:         time.Date(2020, 8, 28, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indices := test.nameProvider.get(test.date)
			assert.Equal(t, test.index, indices)
		})
	}
}
