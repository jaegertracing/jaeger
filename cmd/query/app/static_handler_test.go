// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package app

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/query/app/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

//go:generate mockery -all -dir ./watcher

func TestNotExistingUiConfig(t *testing.T) {
	handler, err := NewStaticAssetsHandler("/foo/bar", StaticAssetsHandlerOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
	assert.Nil(t, handler)
}

func TestRegisterStaticHandlerPanic(t *testing.T) {
	logger, buf := testutils.NewLogger()
	assert.Panics(t, func() {
		RegisterStaticHandler(mux.NewRouter(), logger, &QueryOptions{StaticAssets: "/foo/bar"})
	})
	assert.Contains(t, buf.String(), "Could not create static assets handler")
	assert.Contains(t, buf.String(), "no such file or directory")
}

func TestRegisterStaticHandler(t *testing.T) {
	testCases := []struct {
		basePath         string // input to the test
		subroute         bool   // should we create a subroute?
		baseURL          string // expected URL prefix
		expectedBaseHTML string // substring to match in the home page
	}{
		{basePath: "", baseURL: "/", expectedBaseHTML: `<base href="/"`},
		{basePath: "/", baseURL: "/", expectedBaseHTML: `<base href="/"`},
		{basePath: "/jaeger", baseURL: "/jaeger/", expectedBaseHTML: `<base href="/jaeger/"`, subroute: true},
	}
	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}
	for _, testCase := range testCases {
		t.Run("basePath="+testCase.basePath, func(t *testing.T) {
			logger, _ := testutils.NewLogger()
			r := mux.NewRouter()
			if testCase.subroute {
				r = r.PathPrefix(testCase.basePath).Subrouter()
			}
			RegisterStaticHandler(r, logger, &QueryOptions{
				StaticAssets: "fixture",
				BasePath:     testCase.basePath,
				UIConfig:     "fixture/ui-config.json",
			})

			server := httptest.NewServer(r)
			defer server.Close()

			httpGet := func(path string) string {
				url := fmt.Sprintf("%s%s%s", server.URL, testCase.baseURL, path)
				resp, err := httpClient.Get(url)
				require.NoError(t, err)
				defer resp.Body.Close()

				respByteArray, err := ioutil.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode, "url: %s, response: %v", url, string(respByteArray))
				return string(respByteArray)
			}

			respString := httpGet(favoriteIcon)
			assert.Contains(t, respString, "Test Favicon") // this text is present in fixtures/favicon.ico

			html := httpGet("") // get home page
			assert.Contains(t, html, `JAEGER_CONFIG = {"x":"y"};`, "actual: %v", html)
			assert.Contains(t, html, `JAEGER_VERSION = {"gitCommit":"","gitVersion":"","buildDate":""};`, "actual: %v", html)
			assert.Contains(t, html, testCase.expectedBaseHTML, "actual: %v", html)

			asset := httpGet("static/asset.txt")
			assert.Contains(t, asset, "some asset", "actual: %v", asset)
		})
	}
}

func TestNewStaticAssetsHandlerErrors(t *testing.T) {
	_, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{UIConfigPath: "fixture/invalid-config"})
	assert.Error(t, err)

	for _, base := range []string{"x", "x/", "/x/"} {
		_, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{UIConfigPath: "fixture/ui-config.json", BasePath: base})
		require.Errorf(t, err, "basePath=%s", base)
		assert.Contains(t, err.Error(), "invalid base path")
	}
}

func TestNewWatcherError(t *testing.T) {
	const totalWatcherAddCalls = 2

	for _, tc := range []struct {
		name                string
		errorOnNthAdd       int
		wantFactoryErr      error
		wantWatcherAddErr   error
		wantWatcherAddCalls int
	}{
		{
			name:           "Factory.NewWatcher error",
			wantFactoryErr: fmt.Errorf("new watcher error"),
		},
		{
			name:                "Watcher.Add first call error",
			errorOnNthAdd:       0,
			wantWatcherAddErr:   fmt.Errorf("add first error"),
			wantWatcherAddCalls: 1,
		},
		{
			name:                "Watcher.Add second call error",
			errorOnNthAdd:       1,
			wantWatcherAddErr:   fmt.Errorf("add second error"),
			wantWatcherAddCalls: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare
			watcher := &mocks.Watcher{}
			for i := 0; i < totalWatcherAddCalls; i++ {
				var err error
				if i == tc.errorOnNthAdd {
					err = tc.wantWatcherAddErr
				}
				watcher.On("Add", mock.Anything).Return(err).Once()
			}
			watcher.On("Events").Return(make(chan fsnotify.Event))
			watcher.On("Errors").Return(make(chan error))

			watcherFactory := &mocks.Factory{}
			watcherFactory.On("NewWatcher").Return(watcher, tc.wantFactoryErr)

			// Test
			_, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{
				UIConfigPath:   "fixture/ui-config-hotreload.json",
				WatcherFactory: watcherFactory,
			})

			// Validate
			if tc.wantFactoryErr != nil {
				assert.EqualError(t, err, tc.wantFactoryErr.Error())
			} else if tc.wantWatcherAddErr != nil {
				assert.EqualError(t, err, tc.wantWatcherAddErr.Error())
			} else {
				assert.NoError(t, err)
			}
			watcher.AssertNumberOfCalls(t, "Add", tc.wantWatcherAddCalls)
		})
	}
}

func TestHotReloadUIConfigTempFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "ui-config-hotreload-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	tmpfile, err := ioutil.TempFile(dir, "*.json")
	require.NoError(t, err)
	tmpFileName := tmpfile.Name()

	content, err := ioutil.ReadFile("fixture/ui-config-hotreload.json")
	require.NoError(t, err)

	err = syncWrite(tmpFileName, content, 0644)
	require.NoError(t, err)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	h, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{
		UIConfigPath: tmpFileName,
		Logger:       logger,
	})
	require.NoError(t, err)

	c := string(h.indexHTML.Load().([]byte))
	assert.Contains(t, c, "About Jaeger")

	newContent := strings.Replace(string(content), "About Jaeger", "About a new Jaeger", 1)
	err = syncWrite(tmpFileName, []byte(newContent), 0644)
	require.NoError(t, err)

	waitUntil(t, func() bool {
		return logObserver.FilterMessage("reloaded UI config").
			FilterField(zap.String("filename", tmpFileName)).Len() > 0
	}, 2000, 10*time.Millisecond, "timed out waiting for the hot reload to kick in")

	i := string(h.indexHTML.Load().([]byte))
	assert.Contains(t, i, "About a new Jaeger", logObserver.All())
}

func TestLoadUIConfig(t *testing.T) {
	type testCase struct {
		configFile    string
		expected      map[string]interface{}
		expectedError string
	}

	run := func(description string, testCase testCase) {
		t.Run(description, func(t *testing.T) {
			config, err := loadUIConfig(testCase.configFile)
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, testCase.expected, config)
		})
	}

	run("no config", testCase{})
	run("invalid config", testCase{
		configFile:    "invalid",
		expectedError: "cannot read UI config file invalid: open invalid: no such file or directory",
	})
	run("unsupported type", testCase{
		configFile:    "fixture/ui-config.toml",
		expectedError: "unrecognized UI config file format fixture/ui-config.toml",
	})
	run("malformed", testCase{
		configFile:    "fixture/ui-config-malformed.json",
		expectedError: "cannot parse UI config file fixture/ui-config-malformed.json: invalid character '=' after object key",
	})
	run("json", testCase{
		configFile: "fixture/ui-config.json",
		expected:   map[string]interface{}{"x": "y"},
	})
	run("json-menu", testCase{
		configFile: "fixture/ui-config-menu.json",
		expected: map[string]interface{}{
			"menu": []interface{}{
				map[string]interface{}{
					"label": "GitHub",
					"url":   "https://github.com/jaegertracing/jaeger",
				},
			},
		},
	})
}

type fakeFile struct {
	os.File
}

func (*fakeFile) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func TestLoadIndexHTMLReadError(t *testing.T) {
	open := func(string) (http.File, error) {
		return &fakeFile{}, nil
	}
	_, err := loadIndexHTML(open)
	require.Error(t, err)
}

func waitUntil(t *testing.T, f func() bool, iterations int, sleepInterval time.Duration, timeoutErrMsg string) {
	for i := 0; i < iterations; i++ {
		if f() {
			return
		}
		time.Sleep(sleepInterval)
	}
	require.Fail(t, timeoutErrMsg)
}

// syncWrite ensures data is written to the given filename and flushed to disk.
// This ensures that any watchers looking for file system changes can be reliably alerted.
func syncWrite(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
