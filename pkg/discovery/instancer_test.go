// Copyright (c) 2017 Uber Technologies, Inc.
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

package discovery

import (
	"sync"
	"testing"

	"github.com/go-kit/kit/sd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstancer(t *testing.T) {
	i := &Instancer{}
	f1 := make(chan sd.Event)
	f2 := make(chan sd.Event)

	i.Register(f1)
	i.Register(f2)
	assert.Len(t, i.observers, 2)

	notification1 := sd.Event{Instances: []string{"x", "y"}}
	notification2 := sd.Event{Instances: []string{"a", "b", "c"}}

	// times 2 because we have two subscribers
	expectedInstances := 2 * (len(notification1.Instances) + len(notification2.Instances))

	wg := sync.WaitGroup{}
	wg.Add(expectedInstances)

	receiver := func(ch chan sd.Event) {
		for event := range ch {
			// count total number of instances received
			for range event.Instances {
				wg.Done()
			}
		}
	}

	go receiver(f1)
	go receiver(f2)

	i.Notify(notification1)
	i.Notify(notification2)

	close(f1)
	close(f2)

	wg.Wait()

	i.Deregister(f1)
	assert.Len(t, i.observers, 1)

	require.False(t, i.stopped)
	i.Stop()
	require.True(t, i.stopped)
	i.Notify(notification1) // For code coverage purposes

	i.Deregister(f2)
	assert.Len(t, i.observers, 0)
}
