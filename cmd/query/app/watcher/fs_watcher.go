package watcher

import "github.com/fsnotify/fsnotify"

// Watcher watches for Events and Errors once a resource is added to the watch list.
// Primarily used for mocking the fsnotify lib.
type Watcher interface {
	Add(name string) error
	Events() chan fsnotify.Event
	Errors() chan error
}

// fsnotifyWatcherWrapper wraps the fsnotify.Watcher and implements Watcher.
type fsnotifyWatcherWrapper struct {
	fsnotifyWatcher *fsnotify.Watcher
}

// Add adds the filename to watch.
func (f *fsnotifyWatcherWrapper) Add(name string) error {
	return f.fsnotifyWatcher.Add(name)
}

// Events returns the fsnotify.Watcher's Events chan.
func (f *fsnotifyWatcherWrapper) Events() chan fsnotify.Event {
	return f.fsnotifyWatcher.Events
}

// Errors returns the fsnotify.Watcher's Errors chan.
func (f *fsnotifyWatcherWrapper) Errors() chan error {
	return f.fsnotifyWatcher.Errors
}

// Factory creates new Watchers.
// Primarily used for mocking the fsnotify lib.
type Factory interface {
	NewWatcher() (Watcher, error)
}

// FsNotifyWatcherFactory implements Factory.
type FsNotifyWatcherFactory struct{}

// NewWatcher creates a new fsnotifyWatcherWrapper, wrapping the fsnotify.Watcher.
func (f *FsNotifyWatcherFactory) NewWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	return &fsnotifyWatcherWrapper{fsnotifyWatcher: w}, err
}
