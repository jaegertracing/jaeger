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
	"sync"
)

// Notifier listens to a service discovery system and notifies registered
// observers of changes in the resource instances. A complete set of instances
// is always provided to the observers.
type Notifier interface {
	Register(chan<- []string)
	Unregister(chan<- []string)
}

// Dispatcher can register/unregister observers and pass notifications to them
type Dispatcher struct {
	sync.Mutex
	observers []chan<- []string
}

// Register adds an observer to the list.
func (d *Dispatcher) Register(ch chan<- []string) {
	d.Unregister(ch)
	d.Lock()
	defer d.Unlock()
	d.observers = append(d.observers, ch)
}

// Unregister removes an observer from the list.
func (d *Dispatcher) Unregister(ch chan<- []string) {
	d.Lock()
	defer d.Unlock()
	for i := range d.observers {
		if ch == d.observers[i] {
			d.observers = append(d.observers[0:i], d.observers[i+1:]...)
			break
		}
	}
}

// Notify sends instances to all Observers
func (d *Dispatcher) Notify(instances []string) {
	d.Lock()
	defer d.Unlock()
	for i := range d.observers {
		d.observers[i] <- instances
	}
}
