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
