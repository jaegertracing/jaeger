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

	"github.com/stretchr/testify/assert"
)

func TestDispatcher(t *testing.T) {
	d := &Dispatcher{}
	f1 := make(chan []string)
	f2 := make(chan []string)

	d.Register(f1)
	d.Register(f2)
	assert.Len(t, d.observers, 2)

	notification1 := []string{"x", "y"}
	notification2 := []string{"a", "b", "c"}

	// times 2 because we have two subscribers
	expectedInstances := 2 * (len(notification1) + len(notification2))

	wg := sync.WaitGroup{}
	wg.Add(expectedInstances)

	receiver := func(ch chan []string) {
		for instances := range ch {
			// count total number of instances received
			for range instances {
				wg.Done()
			}
		}
	}

	go receiver(f1)
	go receiver(f2)

	d.Notify(notification1)
	d.Notify(notification2)

	close(f1)
	close(f2)

	wg.Wait()

	d.Unregister(f1)
	assert.Len(t, d.observers, 1)

	d.Unregister(f2)
	assert.Len(t, d.observers, 0)
}
