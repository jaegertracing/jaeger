// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package fswatcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func createTestFiles(t *testing.T) (file1 string, file2 string, file3 string) {
	testDir1 := t.TempDir()

	file1 = filepath.Join(testDir1, "test1.doc")
	err := os.WriteFile(file1, []byte("test data"), 0o600)
	require.NoError(t, err)

	file2 = filepath.Join(testDir1, "test2.doc")
	err = os.WriteFile(file2, []byte("test data"), 0o600)
	require.NoError(t, err)

	testDir2 := t.TempDir()

	file3 = filepath.Join(testDir2, "test3.doc")
	err = os.WriteFile(file3, []byte("test data"), 0o600)
	require.NoError(t, err)

	return file1, file2, file3
}

func TestFSWatcherAddFiles(t *testing.T) {
	file1, file2, file3 := createTestFiles(t)

	// Add one unreadable file
	_, err := New([]string{"invalid-file-name"}, nil, nil)
	require.Error(t, err)

	// Add one readable file
	w, err := New([]string{file1}, nil, nil)
	require.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	require.NoError(t, w.Close())

	// Add one empty-name file and one readable file
	w, err = New([]string{"", file1}, nil, nil)
	require.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	require.NoError(t, w.Close())

	// Add one readable file and one unreadable file
	_, err = New([]string{file1, "invalid-file-name"}, nil, nil)
	require.Error(t, err)

	// Add two readable files from one dir
	w, err = New([]string{file1, file2}, nil, nil)
	require.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	require.NoError(t, w.Close())

	// Add two readable files from two different repos
	w, err = New([]string{file1, file3}, nil, nil)
	require.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	require.NoError(t, w.Close())
}

func TestFSWatcherWithMultipleFiles(t *testing.T) {
	tempDir := t.TempDir()
	testFile1, err := os.Create(tempDir + "test-file-1")
	require.NoError(t, err)
	defer testFile1.Close()

	testFile2, err := os.Create(tempDir + "test-file-2")
	require.NoError(t, err)
	defer testFile2.Close()

	_, err = testFile1.WriteString("test content 1")
	require.NoError(t, err)

	_, err = testFile2.WriteString("test content 2")
	require.NoError(t, err)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)

	onChangeCalled := make(chan struct{}, 10) // Buffered channel to prevent blocking
	onChange := func() {
		logger.Info("Change happens")
		select {
		case onChangeCalled <- struct{}{}:
		default:
		}
	}

	w, err := New([]string{testFile1.Name(), testFile2.Name()}, onChange, logger)
	require.NoError(t, err)
	require.IsType(t, &FSWatcher{}, w)
	t.Cleanup(func() {
		w.Close()
	})

	// Test Write event
	testFile1.WriteString(" changed")
	testFile2.WriteString(" changed")
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Received event").Len() > 0
		},
		"Unable to locate 'Received event' in log. All logs: %v", logObserver)
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Change happens").Len() > 0
		},
		"Unable to locate 'Change happens' in log. All logs: %v", logObserver)
	newHash1, err := hashFile(testFile1.Name())
	require.NoError(t, err)
	newHash2, err := hashFile(testFile2.Name())
	require.NoError(t, err)
	w.mu.RLock()
	assert.Equal(t, newHash1, w.fileHashContentMap[testFile1.Name()])
	assert.Equal(t, newHash2, w.fileHashContentMap[testFile2.Name()])
	w.mu.RUnlock()

	// Test Remove event
	redactedPath1 := redactPath(testFile1.Name())
	redactedPath2 := redactPath(testFile2.Name())

	os.Remove(testFile1.Name())
	os.Remove(testFile2.Name())

	// Check for received events with redacted paths
	assertLogs(t,
		func() bool {
			for _, entry := range logObserver.All() {
				if entry.Message == "Received event" {
					for _, field := range entry.Context {
						if field.Key == "event" {
							eventStr := field.String
							if strings.Contains(eventStr, redactedPath1) || strings.Contains(eventStr, redactedPath2) {
								return true
							}
						}
					}
				}
			}
			return false
		},
		"Unable to locate 'Received event' with redacted paths in log. All logs: %v", logObserver)

	// Check for file read errors with redacted paths
	assertLogs(t,
		func() bool {
			for _, entry := range logObserver.All() {
				if entry.Message == "Unable to read the file" {
					for _, field := range entry.Context {
						if field.Key == "file" {
							filePath := field.String
							if filePath == redactedPath1 || filePath == redactedPath2 {
								return true
							}
						}
					}
				}
			}
			return false
		},
		"Unable to locate 'Unable to read the file' with redacted paths in log. All logs: %v", logObserver)
}

func TestFSWatcherWithSymlinkAndRepoChanges(t *testing.T) {
	testDir := t.TempDir()

	err := os.Symlink("..timestamp-1", filepath.Join(testDir, "..data"))
	require.NoError(t, err)
	err = os.Symlink(filepath.Join("..data", "test.doc"), filepath.Join(testDir, "test.doc"))
	require.NoError(t, err)

	timestamp1Dir := filepath.Join(testDir, "..timestamp-1")
	createTimestampDir(t, timestamp1Dir)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)

	onChangeCalled := make(chan struct{}, 10) // Buffered channel to prevent blocking
	onChange := func() {
		select {
		case onChangeCalled <- struct{}{}:
		default:
		}
	}

	w, err := New([]string{filepath.Join(testDir, "test.doc")}, onChange, logger)
	require.NoError(t, err)
	require.IsType(t, &FSWatcher{}, w)
	t.Cleanup(func() {
		w.Close()
	})

	timestamp2Dir := filepath.Join(testDir, "..timestamp-2")
	createTimestampDir(t, timestamp2Dir)

	err = os.Symlink("..timestamp-2", filepath.Join(testDir, "..data_tmp"))
	require.NoError(t, err)

	os.Rename(filepath.Join(testDir, "..data_tmp"), filepath.Join(testDir, "..data"))
	require.NoError(t, err)
	err = os.RemoveAll(timestamp1Dir)
	require.NoError(t, err)

	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Received event").Len() > 0
		},
		"Unable to locate 'Received event' in log. All logs: %v", logObserver)
	byteData, err := os.ReadFile(filepath.Join(testDir, "test.doc"))
	require.NoError(t, err)
	assert.Equal(t, byteData, []byte("test data"))

	timestamp3Dir := filepath.Join(testDir, "..timestamp-3")
	createTimestampDir(t, timestamp3Dir)
	err = os.Symlink("..timestamp-3", filepath.Join(testDir, "..data_tmp"))
	require.NoError(t, err)
	os.Rename(filepath.Join(testDir, "..data_tmp"), filepath.Join(testDir, "..data"))
	require.NoError(t, err)
	err = os.RemoveAll(timestamp2Dir)
	require.NoError(t, err)

	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Received event").Len() > 0
		},
		"Unable to locate 'Received event' in log. All logs: %v", logObserver)
	byteData, err = os.ReadFile(filepath.Join(testDir, "test.doc"))
	require.NoError(t, err)
	assert.Equal(t, byteData, []byte("test data"))
}

func createTimestampDir(t *testing.T, dir string) {
	t.Helper()
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "test.doc"), []byte("test data"), 0o600)
	require.NoError(t, err)
}

type delayedFormat struct {
	fn func() any
}

func (df delayedFormat) String() string {
	return fmt.Sprintf("%v", df.fn())
}

func assertLogs(t *testing.T, f func() bool, errorMsg string, logObserver *observer.ObservedLogs) {
	assert.Eventuallyf(t, f,
		10*time.Second, 10*time.Millisecond,
		errorMsg,
		delayedFormat{
			fn: func() any { return logObserver.All() },
		},
	)
}

func TestRedactPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "file without extension",
			path:     "/path/to/secret",
			expected: filepath.Join("/path/to", hashFilename("secret")),
		},
		{
			name:     "file with extension",
			path:     "/path/to/secret.txt",
			expected: filepath.Join("/path/to", hashFilename("secret.txt")),
		},
		{
			name:     "hidden file",
			path:     "/path/to/.secret",
			expected: filepath.Join("/path/to", hashFilename(".secret")),
		},
		{
			name:     "just filename",
			path:     "secret.txt",
			expected: hashFilename("secret.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactPath(tt.path)
			assert.Equal(t, tt.expected, result, "redacted path does not match expected")
		})
	}
}

// hashFilename mimics the hashing logic in redactPath for test assertions
func hashFilename(filename string) string {
	if filename == "" {
		return ""
	}
	h := sha256.Sum256([]byte(filename))
	ext := filepath.Ext(filename)
	hashPrefix := hex.EncodeToString(h[:4])
	if ext != "" {
		return hashPrefix + "..." + ext
	}
	return hashPrefix
}

func TestRedactionInLogs(t *testing.T) {
	// Setup test file
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "secret-password.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o600)
	require.NoError(t, err)

	// Setup logger with observer
	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)

	// Create a channel to signal when onChange is called
	onChangeCalled := make(chan struct{})
	onChange := func() {
		select {
		case onChangeCalled <- struct{}{}:
		default:
		}
	}

	// Create and defer cleanup of watcher
	watcher, err := New([]string{testFile}, onChange, logger)
	require.NoError(t, err)
	defer watcher.Close()

	// Modify the file to trigger the watcher
	err = os.WriteFile(testFile, []byte("new content"), 0o600)
	require.NoError(t, err)

	// Wait for the event to be processed or timeout after a second
	select {
	case <-onChangeCalled:
		// onChange was called, continue with assertions
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for onChange to be called")
	}

	// Check that the log contains redacted path
	found := false
	redactedName := redactPath(testFile)

	for _, entry := range logObserver.All() {
		if entry.Message == "Received event" {
			for _, field := range entry.Context {
				if field.Key == "event" {
					eventStr := field.String
					// The log should contain the redacted path
					assert.Contains(t, eventStr, redactedName, "log should contain redacted filename")
					// The log should not contain the original filename
					assert.NotContains(t, eventStr, filepath.Base(testFile), "log should not contain original filename")
					found = true
				}
			}
		}
	}
	assert.True(t, found, "expected log entry not found")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
