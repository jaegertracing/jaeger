// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

// withCaps builds a provider that always reports caps.
func withCaps(caps BackendCapabilities) BackendCapabilityProvider {
	return func(context.Context) BackendCapabilities { return caps }
}

func TestNotExistingUiConfig(t *testing.T) {
	handler, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "/foo/bar"}},
		nilBackendCaps,
		zap.NewNop(),
	)
	require.ErrorContains(t, err, "no such file or directory")
	assert.Nil(t, handler)
}

func TestRegisterStaticHandlerPanic(t *testing.T) {
	logger, buf := testutils.NewLogger()
	assert.Panics(t, func() {
		closer := RegisterStaticHandler(
			http.NewServeMux(),
			logger,
			&QueryOptions{
				UIConfig: UIConfig{
					AssetsPath: "/foo/bar",
				},
			},
			nilBackendCaps,
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
		metricsStorage              bool   // metrics storage enabled?
		logAccess                   bool
		UIConfigPath                string // path to UI config
		expectedUIConfig            string // expected UI config
		expectedBackendCapabilities string // expected backend capabilities blob
	}{
		{
			basePath:                    "",
			baseURL:                     "/",
			archiveStorage:              false,
			logAccess:                   true,
			UIConfigPath:                "",
			expectedUIConfig:            "JAEGER_CONFIG=DEFAULT_CONFIG;",
			expectedBackendCapabilities: `JAEGER_BACKEND_CAPABILITIES = {"archiveStorage":false,"metricsStorage":false,"searchWithoutServiceName":false,"aiAssistant":false};`,
		},
		{
			basePath:                    "/",
			baseURL:                     "/",
			archiveStorage:              false,
			UIConfigPath:                "fixture/ui-config.json",
			expectedUIConfig:            `JAEGER_CONFIG = {"x":"y"};`,
			expectedBackendCapabilities: `JAEGER_BACKEND_CAPABILITIES = {"archiveStorage":false,"metricsStorage":false,"searchWithoutServiceName":false,"aiAssistant":false};`,
		},
		{
			basePath:                    "/jaeger",
			baseURL:                     "/jaeger/",
			subroute:                    true,
			archiveStorage:              true,
			UIConfigPath:                "fixture/ui-config.js",
			expectedUIConfig:            "function UIConfig(){",
			expectedBackendCapabilities: `JAEGER_BACKEND_CAPABILITIES = {"archiveStorage":true,"metricsStorage":false,"searchWithoutServiceName":false,"aiAssistant":false};`,
		},
		{
			basePath:                    "/metrics",
			baseURL:                     "/metrics/",
			subroute:                    true,
			metricsStorage:              true,
			UIConfigPath:                "fixture/ui-config.js",
			expectedUIConfig:            "function UIConfig(){",
			expectedBackendCapabilities: `JAEGER_BACKEND_CAPABILITIES = {"archiveStorage":false,"metricsStorage":true,"searchWithoutServiceName":false,"aiAssistant":false};`,
		},
	}
	httpClient = &http.Client{
		Timeout: 2 * time.Second,
	}
	for _, testCase := range testCases {
		t.Run("basePath="+testCase.basePath, func(t *testing.T) {
			logger, logBuf := testutils.NewLogger()
			r := http.NewServeMux()
			closer := RegisterStaticHandler(
				r, logger, &QueryOptions{
					UIConfig: UIConfig{
						ConfigFile: testCase.UIConfigPath,
						AssetsPath: "fixture",
						LogAccess:  testCase.logAccess,
					},
					BasePath: testCase.basePath,
				},
				withCaps(BackendCapabilities{
					ArchiveStorage: testCase.archiveStorage,
					MetricsStorage: testCase.metricsStorage,
				}),
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
			assert.Contains(t, html, testCase.expectedBackendCapabilities, "actual: %v", html)
			assert.Contains(t, html, `JAEGER_VERSION = {"gitCommit":"","gitVersion":"dev","buildDate":""};`, "actual: %v", html)
			// Verify server-side <base href="..."> injection (ADR-009).
			assert.Contains(t, html, fmt.Sprintf(`<base href=%q data-inject-target="BASE_URL" />`, testCase.baseURL))

			// Verify SPA deep-links serve HTML with 200 OK
			spaURL := fmt.Sprintf("%s%strace/12345", server.URL, testCase.baseURL)
			spaResp, err := httpClient.Get(spaURL)
			require.NoError(t, err)
			defer spaResp.Body.Close()
			assert.Equal(t, http.StatusOK, spaResp.StatusCode)
			assert.Equal(t, "text/html; charset=utf-8", spaResp.Header.Get("Content-Type"))

			// Verify static asset requests via deep link return 404 (and NOT 200 OK with HTML content)
			missingAssetURL := fmt.Sprintf("%s%strace/12345/static/missing.js", server.URL, testCase.baseURL)
			missingResp, err := httpClient.Get(missingAssetURL)
			require.NoError(t, err)
			defer missingResp.Body.Close()
			assert.Equal(t, http.StatusNotFound, missingResp.StatusCode)
			assert.NotEqual(t, "text/html; charset=utf-8", missingResp.Header.Get("Content-Type"))

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

// TestStaticHandlerInjectsBackendCapabilities covers the handler's whole part in this:
// serialize what the provider reports into the JAEGER_BACKEND_CAPABILITIES slot, under the
// key names and in the order the UI bundle expects.
func TestStaticHandlerInjectsBackendCapabilities(t *testing.T) {
	tests := []struct {
		name string
		caps BackendCapabilities
	}{
		{
			name: "nothing supported",
			caps: BackendCapabilities{},
		},
		{
			name: "everything supported",
			caps: BackendCapabilities{
				ArchiveStorage:           true,
				MetricsStorage:           true,
				SearchWithoutServiceName: true,
				AIAssistant:              true,
			},
		},
		{
			name: "one flag does not imply another",
			caps: BackendCapabilities{SearchWithoutServiceName: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := http.NewServeMux()
			closer := RegisterStaticHandler(
				r, zap.NewNop(),
				&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}, BasePath: ""},
				withCaps(tt.caps),
			)
			defer closer.Close()

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
			body := rr.Body.String()

			expected := fmt.Sprintf(
				`JAEGER_BACKEND_CAPABILITIES = {"archiveStorage":%t,"metricsStorage":%t,"searchWithoutServiceName":%t,"aiAssistant":%t};`,
				tt.caps.ArchiveStorage, tt.caps.MetricsStorage,
				tt.caps.SearchWithoutServiceName, tt.caps.AIAssistant,
			)
			assert.Contains(t, body, expected, "backend capabilities injection mismatch")
		})
	}
}

func TestStaticHandlerReflectsLatestAIHealthCheckPerRequest(t *testing.T) {
	var available atomic.Bool
	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}, BasePath: ""},
		func(context.Context) BackendCapabilities {
			return BackendCapabilities{AIAssistant: available.Load()}
		},
	)
	defer closer.Close()

	getBody := func() string {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		return rr.Body.String()
	}

	require.Contains(t, getBody(), `"aiAssistant":false`)

	available.Store(true)
	require.Contains(t, getBody(), `"aiAssistant":true`,
		"next response must reflect the new AI health check value")

	available.Store(false)
	require.Contains(t, getBody(), `"aiAssistant":false`,
		"next response must reflect the new AI health check value")
}

// TestStaticHandlerReflectsSearchCapabilityPerRequest: a page loaded after the backend
// becomes reachable must offer "All Services" without a restart (RFC 0013 §3.6).
func TestStaticHandlerReflectsSearchCapabilityPerRequest(t *testing.T) {
	var supported atomic.Bool
	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}, BasePath: ""},
		func(context.Context) BackendCapabilities {
			return BackendCapabilities{SearchWithoutServiceName: supported.Load()}
		},
	)
	defer closer.Close()

	getBody := func() string {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		return rr.Body.String()
	}

	require.Contains(t, getBody(), `"searchWithoutServiceName":false`)

	supported.Store(true)
	require.Contains(t, getBody(), `"searchWithoutServiceName":true`,
		"a backend that has since answered must be reflected without a restart")
}

func TestNewStaticAssetsHandlerErrors(t *testing.T) {
	_, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture", ConfigFile: "fixture/invalid-config"}},
		nilBackendCaps,
		zap.NewNop(),
	)
	require.Error(t, err)
}

func TestHotReloadUIConfig(t *testing.T) {
	// Shorten the reload TTL so the test doesn't have to wait the full default.
	prev := uiConfigReloadInterval
	uiConfigReloadInterval = 10 * time.Millisecond
	t.Cleanup(func() { uiConfigReloadInterval = prev })

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

	h, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture", ConfigFile: cfgFileName}},
		nilBackendCaps,
		zap.NewNop(),
	)
	require.NoError(t, err)
	defer h.Close()

	assert.Contains(t, string(h.deriveIndexHTML(context.Background())), "About Jaeger")

	newContent := strings.Replace(string(content), "About Jaeger", "About a new Jaeger", 1)
	err = syncWrite(cfgFile, tmpFile, []byte(newContent))
	require.NoError(t, err)

	// The TTL on the cached UI config is 10ms; deriveIndexHTML re-reads from
	// disk after that. Poll until the new content is picked up.
	require.Eventually(t, func() bool {
		return strings.Contains(string(h.deriveIndexHTML(context.Background())), "About a new Jaeger")
	}, time.Second, 5*time.Millisecond, "TTL-based reload did not pick up the new UI config content")
}

func TestUIConfigReloadErrorKeepsCachedValue(t *testing.T) {
	prev := uiConfigReloadInterval
	uiConfigReloadInterval = 10 * time.Millisecond
	t.Cleanup(func() { uiConfigReloadInterval = prev })

	dir := t.TempDir()
	cfgFile, err := os.CreateTemp(dir, "*.json")
	require.NoError(t, err)
	defer cfgFile.Close()
	cfgFileName := cfgFile.Name()

	tmpFile, err := os.CreateTemp(dir, "*.json")
	require.NoError(t, err)
	defer tmpFile.Close()

	good, err := os.ReadFile("fixture/ui-config-hotreload.json")
	require.NoError(t, err)
	require.NoError(t, syncWrite(cfgFile, tmpFile, good))

	zcore, logObserver := observer.New(zapcore.ErrorLevel)
	h, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture", ConfigFile: cfgFileName}},
		nilBackendCaps,
		zap.New(zcore),
	)
	require.NoError(t, err)
	defer h.Close()

	// Overwrite the file with content that fails to parse, then wait past
	// the TTL so the next deriveIndexHTML triggers a reload.
	require.NoError(t, syncWrite(cfgFile, tmpFile, []byte("{not valid json")))

	require.Eventually(t, func() bool {
		// Force a reload attempt by calling deriveIndexHTML; the malformed
		// file should produce a logged error while leaving the previously
		// cached "About Jaeger" content intact.
		body := string(h.deriveIndexHTML(context.Background()))
		if !strings.Contains(body, "About Jaeger") {
			return false
		}
		return logObserver.FilterMessage("could not reload UI config; keeping previously cached value").Len() > 0
	}, time.Second, 5*time.Millisecond,
		"expected the static handler to keep the previously cached UI config when a reload fails")
}

func TestLoadUIConfig(t *testing.T) {
	type testCase struct {
		configFile      string
		expected        *loadedConfig
		expectedError   string
		expectedContent string
	}

	readJSFixture := func(name string) []byte {
		content, err := os.ReadFile(name)
		require.NoError(t, err)
		return bytes.TrimSpace(content)
	}

	run := func(description string, testCase testCase) {
		t.Run(description, func(t *testing.T) {
			config, err := loadUIConfig(testCase.configFile)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, testCase.expected, config)

			if testCase.expectedContent != "" {
				h, err := newStaticAssetsHandler(
					&QueryOptions{UIConfig: UIConfig{ConfigFile: testCase.configFile}},
					nilBackendCaps,
					zap.NewNop(),
				)
				require.NoError(t, err)
				defer h.Close()
				assert.Contains(t, string(h.deriveIndexHTML(context.Background())), testCase.expectedContent)
			}
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
			config: readJSFixture("fixture/ui-config.js"),
		},
		expectedContent: "function UIConfig()",
	})
	run("js-menu", testCase{
		configFile: "fixture/ui-config-menu.js",
		expected: &loadedConfig{
			regexp: configJsPattern,
			config: readJSFixture("fixture/ui-config-menu.js"),
		},
		expectedContent: "function UIConfig()",
	})
}

type fakeFile struct {
	os.File
}

func (*fakeFile) Read([]byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestLoadIndexHTMLReadError(t *testing.T) {
	open := func(string) (http.File, error) {
		return &fakeFile{}, nil
	}
	_, err := loadIndexHTML(open)
	require.Error(t, err)
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

func TestServeSPAStaticAssetGuard(t *testing.T) {
	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}, BasePath: ""},
		nilBackendCaps,
	)
	defer closer.Close()

	tests := []struct {
		path         string
		expectedCode int
		isHTML       bool
	}{
		{path: "/trace/12345", expectedCode: http.StatusOK, isHTML: true},
		{path: "/search", expectedCode: http.StatusOK, isHTML: true},
		{path: "/trace/12345/static/missing.js", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/trace/12345/static/bundle.css", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/trace/static/chunk.js", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/trace/12345.js", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/favicon.ico", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/bundle.map", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/logo.svg", expectedCode: http.StatusNotFound, isHTML: false},
		{path: "/trace/12345/static", expectedCode: http.StatusNotFound, isHTML: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)
			r.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedCode, rr.Code)
			if tt.isHTML {
				assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
				assert.Contains(t, rr.Body.String(), "<!DOCTYPE html>")
			} else {
				assert.NotEqual(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
			}
		})
	}
}

func TestBaseHrefInjectionPriorToAssetLinks(t *testing.T) {
	tempDir := t.TempDir()
	indexContent := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <base href="/" data-inject-target="BASE_URL" />
    <link rel="stylesheet" href="./static/main.css">
    <script type="module" src="./static/main.js"></script>
</head>
<body>
    <div id="root"></div>
</body>
</html>`
	require.NoError(t, os.WriteFile(tempDir+"/index.html", []byte(indexContent), 0o644))

	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: tempDir}, BasePath: "/jaeger"},
		nilBackendCaps,
	)
	defer closer.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jaeger/", http.NoBody)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	baseIdx := strings.Index(body, `<base href="/jaeger/" data-inject-target="BASE_URL" />`)
	linkIdx := strings.Index(body, `<link rel="stylesheet"`)
	scriptIdx := strings.Index(body, `<script type="module"`)

	require.GreaterOrEqual(t, baseIdx, 0, "base tag must be injected")
	require.Greater(t, linkIdx, baseIdx, "base tag must precede link tags")
	require.Greater(t, scriptIdx, baseIdx, "base tag must precede script tags")
}

func TestBaseHrefInjectionWithoutExistingBaseTag(t *testing.T) {
	tempDir := t.TempDir()
	indexContent := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <link rel="stylesheet" href="./static/main.css">
</head>
<body>
    <div id="root"></div>
</body>
</html>`
	require.NoError(t, os.WriteFile(tempDir+"/index.html", []byte(indexContent), 0o644))

	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: tempDir}, BasePath: "/custom/prefix"},
		nilBackendCaps,
	)
	defer closer.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/custom/prefix/", http.NoBody)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	baseIdx := strings.Index(body, `<base href="/custom/prefix/" data-inject-target="BASE_URL" />`)
	linkIdx := strings.Index(body, `<link rel="stylesheet"`)

	require.GreaterOrEqual(t, baseIdx, 0, "base tag must be injected right after head")
	require.Greater(t, linkIdx, baseIdx, "base tag must precede link tags")
}
