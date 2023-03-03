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

type FSWatcher struct {
	watcher *fsnotify.Watcher
	logger  *zap.Logger
}

// FSWatcher waits for notifications of changes in the watched directories
// and attempts to reload all files that changed.
//
// Write and Rename events indicate that some files might have changed and reload might be necessary.
// Remove event indicates that the file was deleted and we should write a warn to log.
//
// Reasoning:
//
// Write event is sent if the file content is rewritten.
//
// Usually files are not rewritten, but they are updated by swapping them with new
// ones by calling Rename. That avoids files being read while they are not yet
// completely written but it also means that inotify on file level will not work:
// watch is invalidated when the old file is deleted.
//
// If reading from Kubernetes Secret volumes the target files are symbolic links
// to files in a different directory. That directory is swapped with a new one,
// while the symbolic links remain the same. This guarantees atomic swap for all
// files at once, but it also means any Rename event in the directory might
// indicate that the files were replaced, even if event.Name is not any of the
// files we are monitoring. We check the hashes of the files to detect if they
// were really changed.
func NewFSWatcher(paths []string, onChange func(), logger *zap.Logger) (*FSWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return &FSWatcher{}, err
	}
	fsw := &FSWatcher{
		watcher: watcher,
		logger:  logger,
	}

	fileHashContentMap := make(map[string]string)
	uniqueDirs := make(map[string]bool)

	for _, p := range paths {
		if p == "" {
			continue
		}
		if h, err := hashFile(p); err == nil {
			fileHashContentMap[p] = h
		} else {
			return &FSWatcher{}, err
		}
		dir := path.Dir(p)
		if _, ok := uniqueDirs[dir]; !ok {
			if err := fsw.watcher.Add(dir); err != nil {
				return &FSWatcher{}, err
			}
			uniqueDirs[dir] = true
		}
	}

	go func() error {
		for {
			select {
			case event := <-fsw.watcher.Events:
				if event.Op&fsnotify.Create == fsnotify.Create ||
					event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Rename == fsnotify.Rename ||
					event.Op&fsnotify.Remove == fsnotify.Remove {
					for file, hash := range fileHashContentMap {
						ok, newHash := fsw.isModified(file, hash)
						if ok {
							fileHashContentMap[file] = newHash
							onChange()
						}
					}
				}
			case err := <-fsw.watcher.Errors:
				return err
			}
		}
	}()

	return fsw, nil
}

// Close closes the watcher.
func (f *FSWatcher) Close() error {
	return f.watcher.Close()
}

// isModified returns true if the file has been modified since the last check.
func (f *FSWatcher) isModified(filepath string, previousHash string) (bool, string) {
	if filepath == "" {
		return false, ""
	}
	hash, err := hashFile(filepath)
	if err != nil {
		f.logger.Warn("File has been removed, using the last known version", zap.String("file", filepath))
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
