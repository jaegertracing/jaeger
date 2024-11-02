// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package fswatcher

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

type FSWatcher struct {
	watcher            *fsnotify.Watcher
	logger             *zap.Logger
	fileHashContentMap map[string]string
	onChange           func()
	mu                 sync.RWMutex
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
func New(filepaths []string, onChange func(), logger *zap.Logger) (*FSWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &FSWatcher{
		watcher:            watcher,
		logger:             logger,
		fileHashContentMap: make(map[string]string),
		onChange:           onChange,
	}

	if err = w.setupWatchedPaths(filepaths); err != nil {
		w.Close()
		return nil, err
	}

	go w.watch()

	return w, nil
}

func (w *FSWatcher) setupWatchedPaths(filepaths []string) error {
	uniqueDirs := make(map[string]bool)

	for _, p := range filepaths {
		if p == "" {
			continue
		}
		h, err := hashFile(p)
		if err != nil {
			return err
		}
		w.fileHashContentMap[p] = h
		dir := path.Dir(p)
		if _, ok := uniqueDirs[dir]; !ok {
			if err := w.watcher.Add(dir); err != nil {
				return err
			}
			uniqueDirs[dir] = true
		}
	}

	return nil
}

// Watch watches for Events and Errors of files.
// Each time an Event happen, all the files are checked for content change.
// If a file's content changes, its hashed content is updated and
// onChange is invoked after all file checks.
func (w *FSWatcher) watch() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.logger.Info("Received event", zap.String("event", event.String()))
			var changed bool
			w.mu.Lock()
			for file, hash := range w.fileHashContentMap {
				fileChanged, newHash := w.isModified(file, hash)
				if fileChanged {
					changed = fileChanged
					w.fileHashContentMap[file] = newHash
				}
			}
			w.mu.Unlock()
			if changed {
				w.onChange()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("fsnotifier reported an error", zap.Error(err))
		}
	}
}

// Close closes the watcher.
func (w *FSWatcher) Close() error {
	return w.watcher.Close()
}

// isModified returns true if the file has been modified since the last check.
func (w *FSWatcher) isModified(filepath string, previousHash string) (bool, string) {
	hash, err := hashFile(filepath)
	if err != nil {
		w.logger.Warn("Unable to read the file", zap.String("file", filepath), zap.Error(err))
		return true, ""
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
