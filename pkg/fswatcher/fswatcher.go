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

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// FSWatcher watches for files' changes and errors.
type FSWatcher interface {
	WatchFiles(paths []string, onChange func(), log *zap.Logger) error
	Close() error
}

// fsWatcherWrapper wraps the fsnotify.Watcher and implements FSWatcher.
type fsWatcherWrapper struct {
	fsnotifyWatcher *fsnotify.Watcher
}

// WatchFiles adds files' directories to the watcher, store each file's hashed content,
// and watch for their changes and errors. If a file's hashed content changes, invoke onChange().
func (f *fsWatcherWrapper) WatchFiles(paths []string, onChange func(), log *zap.Logger) error {
	fileHashContentMap := make(map[string]string)
	uniqueDirs := make(map[string]bool)

	go func() error {
		for {
			select {
			case event := <-f.fsnotifyWatcher.Events:
				if event.Op&fsnotify.Create == fsnotify.Create ||
					event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Rename == fsnotify.Rename {
					ok, newHash := isModified(event.Name, fileHashContentMap[event.Name])
					if ok {
						fileHashContentMap[event.Name] = newHash
						onChange()
					}
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					if log == nil {
						log = zap.NewNop()
					}
					log.Warn(event.Name + "has been removed.")
				}
			case err := <-f.fsnotifyWatcher.Errors:
				return err
			}
		}
	}()

	for _, p := range paths {
		if p == "" {
			continue
		}
		if h, err := hashFile(p); err == nil {
			fileHashContentMap[p] = h
		} else {
			return err
		}
		dir := path.Dir(p)
		if _, ok := uniqueDirs[dir]; !ok {
			if err := f.fsnotifyWatcher.Add(dir); err != nil {
				return err
			}
			uniqueDirs[dir] = true
		}
	}

	return nil
}

// Close closes the watcher.
func (f *fsWatcherWrapper) Close() error {
	return f.fsnotifyWatcher.Close()
}

// NewFSWatcher creates a new fsWatcherWrapper, wrapping the fsnotify.Watcher.
func NewFSWatcher() (FSWatcher, error) {
	w, err := fsnotify.NewWatcher()
	return &fsWatcherWrapper{fsnotifyWatcher: w}, err
}

// isModified returns true if the file has been modified since the last check.
func isModified(file string, previousHash string) (bool, string) {
	if file == "" {
		return false, ""
	}
	hash, err := hashFile(file)
	if err != nil {
		return false, ""
	}
	return previousHash != hash, hash
}

// hashFile returns the SHA256 hash of the file.
func hashFile(file string) (string, error) {
	f, err := os.Open(filepath.Clean(file))
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
