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

type Watcher interface {
	WatchFiles(paths []string, onChange func(), onRemove func()) error
	Close() error
}

// fsnotifyWatcherWrapper wraps the fsnotify.Watcher and implements Watcher.
type fsnotifyWatcherWrapper struct {
	fsnotifyWatcher *fsnotify.Watcher
}

func (f *fsnotifyWatcherWrapper) WatchFiles(paths []string, onChange func(), onRemove func()) error {
	go func() error {
		for {
			select {
			case event := <-f.fsnotifyWatcher.Events:
				if event.Op&fsnotify.Create == fsnotify.Create ||
					event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Rename == fsnotify.Rename {
					onChange()
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					onRemove()
				}
			case err := <-f.fsnotifyWatcher.Errors:
				return err
			}
		}
	}()

	for _, p := range paths {
		if err := f.fsnotifyWatcher.Add(p); err != nil {
			return err
		}
	}

	return nil
}

// Close closes the watcher.
func (f *fsnotifyWatcherWrapper) Close() error {
	return f.fsnotifyWatcher.Close()
}

// NewWatcher creates a new fsnotifyWatcherWrapper, wrapping the fsnotify.Watcher.
func NewWatcher() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	return &fsnotifyWatcherWrapper{fsnotifyWatcher: w}, err
}
