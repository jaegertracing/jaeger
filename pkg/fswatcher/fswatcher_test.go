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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestFSWatcherAddFiles(t *testing.T) {
	// Add one unreadable file
	_, err := NewFSWatcher([]string{"foo"}, nil, nil)
	assert.Error(t, err)

	// Add one readable file
	w, err := NewFSWatcher([]string{"../../cmd/query/app/fixture/ui-config.json"}, nil, nil)
	assert.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	assert.NoError(t, w.Close())

	// Add one readable file and one unreadable file
	_, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/ui-config.json", "foo"}, nil, nil)
	assert.Error(t, err)

	// Add two readable files from one repo
	w, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/ui-config.json", "../../cmd/query/app/fixture/ui-config.toml"}, nil, nil)
	assert.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	assert.NoError(t, w.Close())

	// Add two readable files from two different repos
	w, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/static/asset.txt", "../../cmd/query/app/fixture/ui-config.json"}, nil, nil)
	assert.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	assert.NoError(t, w.Close())
}

func TestFSWatcherWithMultipleFiles(t *testing.T) {
	testFile1, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer testFile1.Close()

	testFile2, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer testFile2.Close()

	_, err = testFile1.WriteString("test content 1")
	require.NoError(t, err)

	_, err = testFile2.WriteString("test content 2")
	require.NoError(t, err)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)

	onChange := func() {
		logger.Info("Change happens")
	}

	w, err := NewFSWatcher([]string{testFile1.Name(), testFile2.Name()}, onChange, logger)
	require.NoError(t, err)
	require.IsType(t, &FSWatcher{}, w)
	defer w.Close()

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
	assert.Equal(t, newHash1, w.fileHashContentMap[testFile1.Name()])
	assert.Equal(t, newHash2, w.fileHashContentMap[testFile2.Name()])

	// Test Remove event
	os.Remove(testFile1.Name())
	os.Remove(testFile2.Name())
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Received event").Len() > 0
		},
		"Unable to locate 'Received event' in log. All logs: %v", logObserver)
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Unable to read the file").FilterField(zap.String("file", testFile1.Name())).Len() > 0
		},
		"Unable to locate 'Unable to read the file' in log. All logs: %v", logObserver)
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Unable to read the file").FilterField(zap.String("file", testFile2.Name())).Len() > 0
		},
		"Unable to locate 'Unable to read the file' in log. All logs: %v", logObserver)
}

func TestFSWatcherWithSymlinkAndRepoChanges(t *testing.T) {
	testDir, err := os.MkdirTemp("", "test")
	require.NoError(t, err)
	defer os.RemoveAll(testDir)

	err = os.Symlink("..timestamp-1", filepath.Join(testDir, "..data"))
	require.NoError(t, err)
	err = os.Symlink(filepath.Join("..data", "test.doc"), filepath.Join(testDir, "test.doc"))
	require.NoError(t, err)

	timestamp1Dir := filepath.Join(testDir, "..timestamp-1")
	createTimestampDir(t, timestamp1Dir)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)

	onChange := func() {}

	w, err := NewFSWatcher([]string{filepath.Join(testDir, "test.doc")}, onChange, logger)
	require.NoError(t, err)
	require.IsType(t, &FSWatcher{}, w)
	defer w.Close()

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
	fn func() interface{}
}

func (df delayedFormat) String() string {
	return fmt.Sprintf("%v", df.fn())
}

func assertLogs(t *testing.T, f func() bool, errorMsg string, logObserver *observer.ObservedLogs) {
	assert.Eventuallyf(t, f,
		10*time.Second, 10*time.Millisecond,
		errorMsg,
		delayedFormat{
			fn: func() interface{} { return logObserver.All() },
		},
	)
}
