// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// getViaAssetsPath spins up a static handler backed by assetsPath and
// returns the response body for route. Used to compare a directory-backed
// deployment against an archive-backed one serving the same bundle.
func getViaAssetsPath(t *testing.T, assetsPath, route string) string {
	t.Helper()
	r := http.NewServeMux()
	closer := RegisterStaticHandler(
		r, zap.NewNop(),
		&QueryOptions{UIConfig: UIConfig{AssetsPath: assetsPath}},
		nilBackendCaps,
	)
	defer closer.Close()

	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + route)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

// TestAssetsFSTarGzServesSameContentAsDirectory covers the archive path's
// main promise: assets_path pointed at fixture.tar.gz must serve the exact
// same bytes as assets_path pointed at the unpacked fixture directory it was
// built from (see fixture.tar.gz's construction, mirroring fixture/).
func TestAssetsFSTarGzServesSameContentAsDirectory(t *testing.T) {
	for _, route := range []string{"/", "/static/asset.txt"} {
		t.Run(route, func(t *testing.T) {
			dirBody := getViaAssetsPath(t, "fixture", route)
			archiveBody := getViaAssetsPath(t, "fixture.tar.gz", route)
			assert.Equal(t, dirBody, archiveBody)
		})
	}
}

// TestAssetsFSTgzSuffix covers the shorter .tgz spelling of the same
// gzip+tar format.
func TestAssetsFSTgzSuffix(t *testing.T) {
	data, err := os.ReadFile("fixture.tar.gz")
	require.NoError(t, err)
	tgzPath := filepath.Join(t.TempDir(), "assets.tgz")
	require.NoError(t, os.WriteFile(tgzPath, data, 0o600))

	fsys, err := newAssetsFS(tgzPath)
	require.NoError(t, err)
	f, err := fsys.Open("/index.html")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestAssetsFSUnsupportedFileSuffix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.zip")
	require.NoError(t, os.WriteFile(path, []byte("not an archive"), 0o600))

	_, err := newAssetsFS(path)
	require.ErrorContains(t, err, "not a .tar.gz or .tgz archive")
}

func TestAssetsFSCorruptedArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.tar.gz")
	require.NoError(t, os.WriteFile(path, []byte("not actually gzip"), 0o600))

	_, err := newAssetsFS(path)
	require.ErrorContains(t, err, "cannot read assets archive")
}

// TestArchiveAssetsFSOpenError covers the case where the archive is removed
// (or otherwise becomes unreadable) between newAssetsFS's os.Stat and the
// actual open — exercised directly since newAssetsFS's own stat prevents
// reaching this path through the public entry point with a merely
// nonexistent file.
func TestArchiveAssetsFSOpenError(t *testing.T) {
	_, err := archiveAssetsFS(filepath.Join(t.TempDir(), "gone.tar.gz"))
	require.ErrorContains(t, err, "cannot open assets archive")
}

// TestAssetsFSMalformedTarHeader covers a stream that gzip decodes cleanly
// but that archive/tar rejects: a 512-byte block that is not a valid tar
// header (and not the all-zero end-of-archive marker either).
func TestAssetsFSMalformedTarHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.tar.gz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	garbage := bytes.Repeat([]byte("X"), 512) // non-zero, fails the tar header checksum
	_, err = gz.Write(garbage)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	_, err = newAssetsFS(path)
	require.ErrorContains(t, err, "cannot read assets archive")
}

// TestAssetsFSSkipsNonRegularEntries covers a directory entry alongside a
// regular file: the directory is skipped rather than added to the served
// tree (fstest.MapFS synthesizes directories from file paths on its own).
func TestAssetsFSSkipsNonRegularEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.tar.gz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "static/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}))
	content := []byte("some asset")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "static/asset.txt",
		Mode: 0o644,
		Size: int64(len(content)),
	}))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	fsys, err := newAssetsFS(path)
	require.NoError(t, err)

	_, err = fsys.Open("/static/")
	assert.Error(t, err, "the directory entry itself must not become a servable file")

	hf, err := fsys.Open("/static/asset.txt")
	require.NoError(t, err)
	defer hf.Close()
	data, err := io.ReadAll(hf)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

// TestAssetsFSTruncatedEntryBody covers a header that parses correctly but
// whose declared Size promises more bytes than the archive actually
// contains, e.g. from a truncated download.
func TestAssetsFSTruncatedEntryBody(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	content := []byte("this content will never actually be written to the archive")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "index.html",
		Mode: 0o644,
		Size: int64(len(content)),
	}))
	// Deliberately not calling tw.Write: the header promises len(content)
	// bytes that never follow, simulating a stream cut short after the
	// header block.
	headerOnly := raw.Bytes()
	require.NotEmpty(t, headerOnly)

	path := filepath.Join(t.TempDir(), "assets.tar.gz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	_, err = gz.Write(headerOnly)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	_, err = newAssetsFS(path)
	require.ErrorContains(t, err, "cannot read index.html from assets archive")
}

func TestAssetsFSEmptyArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.tar.gz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	_, err = newAssetsFS(path)
	require.ErrorContains(t, err, "contains no files")
}

// TestAssetsFSSanitizesTraversalNames confirms an archive entry name that
// climbs above the root (e.g. from a maliciously or accidentally built
// archive) is neutralized to a path under the served tree rather than
// escaping it or erroring out.
func TestAssetsFSSanitizesTraversalNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.tar.gz")
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	content := []byte("hi")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "../../etc/passwd",
		Mode: 0o644,
		Size: int64(len(content)),
	}))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	fsys, err := newAssetsFS(path)
	require.NoError(t, err)

	hf, err := fsys.Open("/etc/passwd")
	require.NoError(t, err)
	defer hf.Close()
	data, err := io.ReadAll(hf)
	require.NoError(t, err)
	assert.Equal(t, content, data)
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
			// Verify the inline base-path script marker is present and the backend
			// did not rewrite <base href> to a path-specific value (ADR-009).
			assert.Contains(t, html, `data-inject-target="BASE_URL"`, "<base> tag must carry data-inject-target marker for client-side base-path detection")
			if testCase.basePath != "" && testCase.basePath != "/" {
				assert.NotContains(t, html, `<base href="`+testCase.baseURL+`"`, "backend must not inject path-specific <base href>")
			}

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
