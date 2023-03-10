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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestFSWatcherAddFiles(t *testing.T) {
	//Add an unreadable file
	w, err := NewFSWatcher([]string{"foo"}, nil, nil)
	assert.Error(t, err)
	assert.IsType(t, &FSWatcher{}, w)

	//Add a readable file
	w, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/ui-config.json"}, nil, nil)
	assert.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	err = w.Close()
	assert.NoError(t, err)

	//Add a readable file and an unreadable file
	w, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/ui-config.json", "foo"}, nil, nil)
	assert.Error(t, err)
	assert.IsType(t, &FSWatcher{}, w)

	//Add two readable files from two different repos
	w, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/static/asset.txt", "../../cmd/query/app/fixture/ui-config.json"}, nil, nil)
	assert.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	err = w.Close()
	assert.NoError(t, err)

	//Add two readable files from one repo
	w, err = NewFSWatcher([]string{"../../cmd/query/app/fixture/ui-config.json", "../../cmd/query/app/fixture/ui-config.toml"}, nil, nil)
	assert.NoError(t, err)
	assert.IsType(t, &FSWatcher{}, w)
	err = w.Close()
	assert.NoError(t, err)
}

func TestFSWatcherWatchFileChangeAndRemove(t *testing.T) {
	testFile, err := os.CreateTemp("", "")
	require.NoError(t, err)

	_, err = testFile.WriteString("test content")
	require.NoError(t, err)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)

	onChange := func() {
		byteContent, err := os.ReadFile(testFile.Name())
		require.NoError(t, err)
		content := string(byteContent)
		logger.Info("Content changed", zap.String("content", content))
	}

	w, err := NewFSWatcher([]string{testFile.Name()}, onChange, logger)
	require.NoError(t, err)
	require.IsType(t, &FSWatcher{}, w)

	testFile.WriteString(" changed")
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Content changed").
				FilterField(zap.String("content", "test content changed")).Len() > 0
		},
		"Unable to locate 'Content changed' in log. All logs: %v", logObserver)

	os.Remove(testFile.Name())
	assertLogs(t,
		func() bool {
			return logObserver.FilterMessage("Unable to read the file").FilterField(zap.String("file", testFile.Name())).Len() > 0
		},
		"Unable to locate 'Unable to read the file' in log. All logs: %v", logObserver)

	err = w.Close()
	assert.NoError(t, err)
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
