// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/ui"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/fswatcher"
	"github.com/jaegertracing/jaeger/internal/version"
)

var (
	// The following patterns are searched and replaced in the index.html as a way of customizing the UI.
	configPattern       = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	configJsPattern     = regexp.MustCompile(`(?im)^\s*//\s*JAEGER_CONFIG_JS.*\n.*`)
	versionPattern      = regexp.MustCompile("JAEGER_VERSION *= *DEFAULT_VERSION;")
	capabilitiesPattern = regexp.MustCompile("JAEGER_BACKEND_CAPABILITIES *= *DEFAULT_BACKEND_CAPABILITIES;")
)

// BackendCapabilities is the JSON shape injected into index.html via the
// JAEGER_BACKEND_CAPABILITIES search-replace pattern. It mirrors the legacy
// storage flags so newer UIs can read everything from a single source, and
// adds the aiAssistant flag the chat surface gates on.
type BackendCapabilities struct {
	ArchiveStorage bool `json:"archiveStorage"`
	MetricsStorage bool `json:"metricsStorage"`
	AIAssistant    bool `json:"aiAssistant"`
}

// RegisterStaticHandler adds handler for static assets to the router.
// aiHealthCheck may be nil; the chat surface stays hidden when it is.
func RegisterStaticHandler(r *http.ServeMux, logger *zap.Logger, qOpts *QueryOptions, qCapabilities querysvc.StorageCapabilities, aiHealthCheck func() bool) io.Closer {
	staticHandler, err := NewStaticAssetsHandler(qOpts.UIConfig.AssetsPath, StaticAssetsHandlerOptions{
		UIConfig:            qOpts.UIConfig,
		BasePath:            qOpts.BasePath,
		StorageCapabilities: qCapabilities,
		AIHealthCheck:       aiHealthCheck,
		Logger:              logger,
	})
	if err != nil {
		logger.Panic("Could not create static assets handler", zap.Error(err))
	}

	staticHandler.RegisterRoutes(r)

	return staticHandler
}

// StaticAssetsHandler handles static assets
type StaticAssetsHandler struct {
	options   StaticAssetsHandlerOptions
	indexHTML atomic.Value // stores []byte
	assetsFS  http.FileSystem
	watcher   *fswatcher.FSWatcher
}

// StaticAssetsHandlerOptions defines options for NewStaticAssetsHandler
type StaticAssetsHandlerOptions struct {
	UIConfig
	BasePath            string
	StorageCapabilities querysvc.StorageCapabilities
	// AIHealthCheck reports whether the AI sidecar is currently reachable.
	// Called on every serve of the SPA root so the injected aiAssistant flag
	// reflects the latest health-check result. nil is treated as "always
	// false" — used when the AI gateway is not configured.
	AIHealthCheck func() bool
	Logger        *zap.Logger
}

type loadedConfig struct {
	regexp *regexp.Regexp
	config []byte
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string, options StaticAssetsHandlerOptions) (*StaticAssetsHandler, error) {
	assetsFS := ui.GetStaticFiles(options.Logger)
	if staticAssetsRoot != "" {
		assetsFS = http.Dir(staticAssetsRoot)
	}

	h := &StaticAssetsHandler{
		options:  options,
		assetsFS: assetsFS,
	}

	indexHTML, err := h.loadAndEnrichIndexHTML(assetsFS.Open)
	if err != nil {
		return nil, err
	}

	options.Logger.Info("Using UI configuration", zap.String("path", options.ConfigFile))
	watcher, err := fswatcher.New([]string{options.ConfigFile}, h.reloadUIConfig, h.options.Logger)
	if err != nil {
		return nil, err
	}
	h.watcher = watcher

	h.indexHTML.Store(indexHTML)

	return h, nil
}

func (sH *StaticAssetsHandler) loadAndEnrichIndexHTML(open func(string) (http.File, error)) ([]byte, error) {
	indexBytes, err := loadIndexHTML(open)
	if err != nil {
		return nil, fmt.Errorf("cannot load index.html: %w", err)
	}
	// replace UI config
	if configObject, err := loadUIConfig(sH.options.ConfigFile); err != nil {
		return nil, err
	} else if configObject != nil {
		indexBytes = configObject.regexp.ReplaceAll(indexBytes, configObject.config)
	}
	// replace Jaeger version
	versionJSON, _ := json.Marshal(version.Get())
	versionString := fmt.Sprintf("JAEGER_VERSION = %s;", string(versionJSON))
	indexBytes = versionPattern.ReplaceAll(indexBytes, []byte(versionString))
	// The <base href> is no longer injected here. The UI detects its own mount-point
	// prefix at page-load time via an inline script in index.html (see ADR-009).
	//
	// Note: JAEGER_BACKEND_CAPABILITIES is intentionally NOT substituted here.
	// The aiAssistant portion of that blob can flip at runtime as the sidecar
	// comes and goes, so the substitution happens per request inside
	// injectBackendCapabilities — the cached HTML keeps the original
	// `JAEGER_BACKEND_CAPABILITIES = DEFAULT_BACKEND_CAPABILITIES;` line
	// untouched, and each response gets a freshly-derived value.

	return indexBytes, nil
}

func (sH *StaticAssetsHandler) reloadUIConfig() {
	sH.options.Logger.Info("reloading UI config", zap.String("filename", sH.options.ConfigFile))
	content, err := sH.loadAndEnrichIndexHTML(sH.assetsFS.Open)
	if err != nil {
		sH.options.Logger.Error("error while reloading the UI config", zap.Error(err))
	}
	sH.indexHTML.Store(content)
	sH.options.Logger.Info("reloaded UI config", zap.String("filename", sH.options.ConfigFile))
}

// injectBackendCapabilities substitutes the JAEGER_BACKEND_CAPABILITIES line in
// the cached index.html with a freshly-derived blob. Called per response so the
// aiAssistant flag reflects the latest health check.
func (sH *StaticAssetsHandler) injectBackendCapabilities(indexBytes []byte) []byte {
	aiAvailable := false
	if sH.options.AIHealthCheck != nil {
		aiAvailable = sH.options.AIHealthCheck()
	}
	backend := BackendCapabilities{
		ArchiveStorage: sH.options.StorageCapabilities.ArchiveStorage,
		MetricsStorage: sH.options.StorageCapabilities.MetricsStorage,
		AIAssistant:    aiAvailable,
	}
	backendJSON, _ := json.Marshal(backend)
	backendString := fmt.Sprintf("JAEGER_BACKEND_CAPABILITIES = %s;", string(backendJSON))
	return capabilitiesPattern.ReplaceAll(indexBytes, []byte(backendString))
}

func loadIndexHTML(open func(string) (http.File, error)) ([]byte, error) {
	indexFile, err := open("/index.html")
	if err != nil {
		return nil, fmt.Errorf("cannot open index.html: %w", err)
	}
	defer indexFile.Close()
	indexBytes, err := io.ReadAll(indexFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read from index.html: %w", err)
	}
	return indexBytes, nil
}

func loadUIConfig(uiConfig string) (*loadedConfig, error) {
	if uiConfig == "" {
		return nil, nil
	}
	bytesConfig, err := os.ReadFile(filepath.Clean(uiConfig))
	if err != nil {
		return nil, fmt.Errorf("cannot read UI config file %v: %w", uiConfig, err)
	}
	var r []byte

	ext := filepath.Ext(uiConfig)
	switch strings.ToLower(ext) {
	case ".json":
		var c map[string]any

		if err := json.Unmarshal(bytesConfig, &c); err != nil {
			return nil, fmt.Errorf("cannot parse UI config file %v: %w", uiConfig, err)
		}
		r, _ = json.Marshal(c)

		return &loadedConfig{
			regexp: configPattern,
			config: append([]byte("JAEGER_CONFIG = "), append(r, byte(';'))...),
		}, nil
	case ".js":
		r = bytes.TrimSpace(bytesConfig)
		re := regexp.MustCompile(`function\s+UIConfig(\s)?\(\s?\)(\s)?{`)
		if !re.Match(r) {
			return nil, fmt.Errorf("UI config file must define function UIConfig(): %v", uiConfig)
		}

		return &loadedConfig{
			regexp: configJsPattern,
			config: r,
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized UI config file format, expecting .js or .json file: %v", uiConfig)
	}
}

func (sH *StaticAssetsHandler) loggingHandler(handler http.Handler) http.Handler {
	if !sH.options.LogAccess {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sH.options.Logger.Info("serving static asset", zap.Stringer("url", r.URL))
		handler.ServeHTTP(w, r)
	})
}

// RegisterRoutes registers routes for this handler on the given router.
func (sH *StaticAssetsHandler) RegisterRoutes(router *http.ServeMux) {
	basePath := sH.options.BasePath
	if basePath == "" {
		basePath = "/"
	}

	fileServer := http.FileServer(sH.assetsFS)
	if basePath != "/" {
		fileServer = http.StripPrefix(basePath+"/", fileServer)
	}

	// Register static files handler
	var staticPattern string
	if basePath == "/" {
		staticPattern = "/static/"
	} else {
		staticPattern = basePath + "/static/"
	}
	router.Handle(staticPattern, sH.loggingHandler(fileServer))

	// Register catch-all handler for SPA routing (serves index.html for all non-API routes).
	// This must be registered last to act as a fallback.
	// Note that the invalid /api/* routes return 404 directly.
	var catchAllPattern string
	if basePath == "/" {
		catchAllPattern = "/"
	} else {
		catchAllPattern = basePath + "/"
	}
	router.Handle(catchAllPattern, sH.loggingHandler(http.HandlerFunc(sH.notFound)))
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(sH.injectBackendCapabilities(sH.indexHTML.Load().([]byte)))
}

func (sH *StaticAssetsHandler) Close() error {
	return sH.watcher.Close()
}
