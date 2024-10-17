// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

//go:generate mockery -all -dir ../../../pkg/fswatcher

func TestNotExistingUiConfig(t *testing.T) {
	handler, err := NewStaticAssetsHandler("/foo/bar", StaticAssetsHandlerOptions{
		Logger: zap.NewNop(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
	assert.Nil(t, handler)
}

func TestRegisterStaticHandlerPanic(t *testing.T) {
	logger, buf := testutils.NewLogger()
	assert.Panics(t, func() {
		closer := RegisterStaticHandler(
			mux.NewRouter(),
			logger,
			&QueryOptions{
				UIConfig: UIConfig{
					AssetsPath: "/foo/bar",
				},
			},
			querysvc.StorageCapabilities{ArchiveStorage: false},
		)
		defer closer.Close()
	})
	assert.Contains(t, buf.String(), "Could not create static assets handler")
	assert.Contains(t, buf.String(), "no such file or directory")
}

func TestRegisterStaticHandler(t *testing.T) {
	testCases := []struct {
		basePath                    string // input to the test
		subroute                    bool   // should we create a subroute?
		baseURL                     string // expected URL prefix
		archiveStorage              bool   // archive storage enabled?
		logAccess                   bool
		expectedBaseHTML            string // substring to match in the home page
		UIConfigPath                string // path to UI config
		expectedUIConfig            string // expected UI config
		expectedStorageCapabilities string // expected storage capabilities
	}{
		{
			basePath:                    "",
			baseURL:                     "/",
			expectedBaseHTML:            `<base href="/"`,
			archiveStorage:              false,
			logAccess:                   true,
			UIConfigPath:                "",
			expectedUIConfig:            "JAEGER_CONFIG=DEFAULT_CONFIG;",
			expectedStorageCapabilities: `JAEGER_STORAGE_CAPABILITIES = {"archiveStorage":false};`,
		},
		{
			basePath:                    "/",
			baseURL:                     "/",
			archiveStorage:              false,
			expectedBaseHTML:            `<base href="/"`,
			UIConfigPath:                "fixture/ui-config.json",
			expectedUIConfig:            `JAEGER_CONFIG = {"x":"y"};`,
			expectedStorageCapabilities: `JAEGER_STORAGE_CAPABILITIES = {"archiveStorage":false};`,
		},
		{
			basePath:                    "/jaeger",
			baseURL:                     "/jaeger/",
			expectedBaseHTML:            `<base href="/jaeger/"`,
			subroute:                    true,
			archiveStorage:              true,
			UIConfigPath:                "fixture/ui-config.js",
			expectedUIConfig:            "function UIConfig(){",
			expectedStorageCapabilities: `JAEGER_STORAGE_CAPABILITIES = {"archiveStorage":true};`,
		},
	}
	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}
	for _, testCase := range testCases {
		t.Run("basePath="+testCase.basePath, func(t *testing.T) {
			logger, logBuf := testutils.NewLogger()
			r := mux.NewRouter()
			if testCase.subroute {
				r = r.PathPrefix(testCase.basePath).Subrouter()
			}
			closer := RegisterStaticHandler(r, logger, &QueryOptions{
				UIConfig: UIConfig{
					ConfigFile: testCase.UIConfigPath,
					AssetsPath: "fixture",
					LogAccess:  testCase.logAccess,
				},
				BasePath: testCase.basePath,
			},
				querysvc.StorageCapabilities{ArchiveStorage: testCase.archiveStorage},
			)
			defer closer.Close()

			server := httptest.NewServer(r)
			defer server.Close()

			httpGet := func(path string) string {
				url := fmt.Sprintf("%s%s%s", server.URL, testCase.baseURL, path)
				resp, err := httpClient.Get(url)
				require.NoError(t, err)
				defer resp.Body.Close()

				respByteArray, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode, "url: %s, response: %v", url, string(respByteArray))
				return string(respByteArray)
			}

			html := httpGet("") // get home page
			assert.Contains(t, html, testCase.expectedUIConfig, "actual: %v", html)
			assert.Contains(t, html, testCase.expectedStorageCapabilities, "actual: %v", html)
			assert.Contains(t, html, `JAEGER_VERSION = {"gitCommit":"","gitVersion":"","buildDate":""};`, "actual: %v", html)
			assert.Contains(t, html, testCase.expectedBaseHTML, "actual: %v", html)

			asset := httpGet("static/asset.txt")
			assert.Contains(t, asset, "some asset", "actual: %v", asset)
			if testCase.logAccess {
				assert.Contains(t, logBuf.String(), "static/asset.txt")
			} else {
				assert.NotContains(t, logBuf.String(), "static/asset.txt")
			}
		})
	}
}

func TestNewStaticAssetsHandlerErrors(t *testing.T) {
	_, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{
		UIConfig: UIConfig{
			ConfigFile: "fixture/invalid-config",
		},
		Logger: zap.NewNop(),
	})
	require.Error(t, err)

	for _, base := range []string{"x", "x/", "/x/"} {
		_, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{
			UIConfig: UIConfig{
				ConfigFile: "fixture/ui-config.json",
			},
			BasePath: base,
			Logger:   zap.NewNop(),
		})
		require.Errorf(t, err, "basePath=%s", base)
		assert.Contains(t, err.Error(), "invalid base path")
	}
}

func TestHotReloadUIConfig(t *testing.T) {
	dir := t.TempDir()

	cfgFile, err := os.CreateTemp(dir, "*.json")
	require.NoError(t, err)
	defer cfgFile.Close()
	cfgFileName := cfgFile.Name()

	tmpFile, err := os.CreateTemp(dir, "*.json")
	require.NoError(t, err)
	defer tmpFile.Close()

	content, err := os.ReadFile("fixture/ui-config-hotreload.json")
	require.NoError(t, err)

	err = syncWrite(cfgFile, tmpFile, content)
	require.NoError(t, err)

	zcore, logObserver := observer.New(zapcore.InfoLevel)
	logger := zap.New(zcore)
	h, err := NewStaticAssetsHandler("fixture", StaticAssetsHandlerOptions{
		UIConfig: UIConfig{
			ConfigFile: cfgFileName,
		},
		Logger: logger,
	})
	require.NoError(t, err)
	defer h.Close()

	c := string(h.indexHTML.Load().([]byte))
	assert.Contains(t, c, "About Jaeger")

	newContent := strings.Replace(string(content), "About Jaeger", "About a new Jaeger", 1)
	err = syncWrite(cfgFile, tmpFile, []byte(newContent))
	require.NoError(t, err)

	waitUntil(t, func() bool {
		return logObserver.FilterMessage("reloaded UI config").
			FilterField(zap.String("filename", cfgFileName)).Len() > 0
	}, 100, 10*time.Millisecond, "timed out waiting for the hot reload to kick in")

	i := string(h.indexHTML.Load().([]byte))
	assert.Contains(t, i, "About a new Jaeger", logObserver.All())
}

func TestLoadUIConfig(t *testing.T) {
	type testCase struct {
		configFile    string
		expected      *loadedConfig
		expectedError string
	}

	run := func(description string, testCase testCase) {
		t.Run(description, func(t *testing.T) {
			config, err := loadUIConfig(testCase.configFile)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
			assert.EqualValues(t, testCase.expected, config)
		})
	}

	run("no config", testCase{})
	run("invalid json config", testCase{
		configFile:    "invalid",
		expectedError: "cannot read UI config file invalid: open invalid: no such file or directory",
	})
	run("unsupported type", testCase{
		configFile:    "fixture/ui-config.toml",
		expectedError: "unrecognized UI config file format, expecting .js or .json file: fixture/ui-config.toml",
	})
	run("malformed", testCase{
		configFile:    "fixture/ui-config-malformed.json",
		expectedError: "cannot parse UI config file fixture/ui-config-malformed.json: invalid character '=' after object key",
	})
	run("json", testCase{
		configFile: "fixture/ui-config.json",
		expected: &loadedConfig{
			config: []byte(`JAEGER_CONFIG = {"x":"y"};`),
			regexp: configPattern,
		},
	})
	c, _ := json.Marshal(map[string]any{
		"menu": []any{
			map[string]any{
				"label": "GitHub",
				"url":   "https://github.com/jaegertracing/jaeger",
			},
		},
	})
	run("json-menu", testCase{
		configFile: "fixture/ui-config-menu.json",
		expected: &loadedConfig{
			config: append([]byte("JAEGER_CONFIG = "), append(c, byte(';'))...),
			regexp: configPattern,
		},
	})
	run("malformed js config", testCase{
		configFile:    "fixture/ui-config-malformed.js",
		expectedError: "UI config file must define function UIConfig(): fixture/ui-config-malformed.js",
	})
	run("js", testCase{
		configFile: "fixture/ui-config.js",
		expected: &loadedConfig{
			regexp: configJsPattern,
			config: []byte(`function UIConfig(){
  return {
    x: "y"
  }
}`),
		},
	})
	run("js-menu", testCase{
		configFile: "fixture/ui-config-menu.js",
		expected: &loadedConfig{
			regexp: configJsPattern,
			config: []byte(`function UIConfig(){
  return {
    menu: [
      {
        label: "GitHub",
        url: "https://github.com/jaegertracing/jaeger"
      }
    ]
  }
}`),
		},
	})
}

type fakeFile struct {
	os.File
}

func (*fakeFile) Read([]byte) (n int, err error) {
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
	t.Helper()
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
func syncWrite(target *os.File, temp *os.File, data []byte) error {
	f, err := os.OpenFile(temp.Name(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), target.Name())
}
