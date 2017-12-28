// Copyright (c) 2017 The Jaeger Authors.
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

	"github.com/go-kit/kit/sd"
)

// Instancer listens to a service discovery system and notifies registered
// observers of changes in the resource instances.
type Instancer struct {
	mux       sync.Mutex
	observers []chan<- sd.Event
	stopped   bool
}

// Register implements sd.Instancer#Register
func (i *Instancer) Register(ch chan<- sd.Event) {
	i.Deregister(ch)
	i.mux.Lock()
	defer i.mux.Unlock()
	i.observers = append(i.observers, ch)
}

// Deregister implements sd.Instancer#Deregister
func (i *Instancer) Deregister(ch chan<- sd.Event) {
	i.mux.Lock()
	defer i.mux.Unlock()
	for j := range i.observers {
		if ch == i.observers[j] {
			i.observers = append(i.observers[0:j], i.observers[j+1:]...)
			break
		}
	}
}

// Stop implements sd.Instancer#Stop
func (i *Instancer) Stop() {
	i.mux.Lock()
	defer i.mux.Unlock()
	i.stopped = true
}

// Notify sends an event to all Observers
func (i *Instancer) Notify(event sd.Event) {
	i.mux.Lock()
	defer i.mux.Unlock()
	if i.stopped {
		return
	}
	for j := range i.observers {
		i.observers[j] <- event
	}
}
