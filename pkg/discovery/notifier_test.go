// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package discovery

import (
	"testing"

	"sync"

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
