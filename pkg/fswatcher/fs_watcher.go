// Copyright (c) 2021 The Jaeger Authors.
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

package fswatcher

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

// NewWatcher creates a new fsnotifyWatcherWrapper, wrapping the fsnotify.Watcher.
func NewWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	return &fsnotifyWatcherWrapper{fsnotifyWatcher: w}, err
}
