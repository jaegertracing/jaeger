// Copyright (c) 2018 The Jaeger Authors.
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

package offset

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
)

func TestHandleReset(t *testing.T) {
	offset := int64(1498)
	minOffset := offset - 1

	m := metrics.NewLocalFactory(0)

	var wg sync.WaitGroup
	wg.Add(1)
	var captureOffset int64
	fakeMarker := func(offset int64) {
		captureOffset = offset
		wg.Done()
	}
	manager := NewManager(minOffset, fakeMarker, 1, m)
	manager.Start()

	manager.MarkOffset(offset)
	wg.Wait()
	manager.Close()

	assert.Equal(t, offset, captureOffset)
	cnt, g := m.Snapshot()
	assert.Equal(t, int64(1), cnt["offset-commits-total|partition=1"])
	assert.Equal(t, int64(offset), g["last-committed-offset|partition=1"])
}
