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

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/cmd/query/app/ui"
	"github.com/jaegertracing/jaeger/pkg/fswatcher"
	"github.com/jaegertracing/jaeger/pkg/version"
)

var (
	// The following patterns are searched and replaced in the index.html as a way of customizing the UI.
	configPattern      = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	configJsPattern    = regexp.MustCompile(`(?im)^\s*\/\/\s*JAEGER_CONFIG_JS.*\n.*`)
	versionPattern     = regexp.MustCompile("JAEGER_VERSION *= *DEFAULT_VERSION;")
	compabilityPattern = regexp.MustCompile("JAEGER_STORAGE_CAPABILITIES *= *DEFAULT_STORAGE_CAPABILITIES;")
	basePathPattern    = regexp.MustCompile(`<base href="/"`) // Note: tag is not closed
)

// RegisterStaticHandler adds handler for static assets to the router.
func RegisterStaticHandler(r *mux.Router, logger *zap.Logger, qOpts *QueryOptions, qCapabilities querysvc.StorageCapabilities) io.Closer {
	staticHandler, err := NewStaticAssetsHandler(qOpts.StaticAssets.Path, StaticAssetsHandlerOptions{
		BasePath:            qOpts.BasePath,
		UIConfigPath:        qOpts.UIConfig,
		StorageCapabilities: qCapabilities,
		Logger:              logger,
		LogAccess:           qOpts.StaticAssets.LogAccess,
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
	BasePath            string
	UIConfigPath        string
	LogAccess           bool
	StorageCapabilities querysvc.StorageCapabilities
	Logger              *zap.Logger
}

type loadedConfig struct {
	regexp *regexp.Regexp
	config []byte
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string, options StaticAssetsHandlerOptions) (*StaticAssetsHandler, error) {
	assetsFS := ui.StaticFiles
	if staticAssetsRoot != "" {
		assetsFS = http.Dir(staticAssetsRoot)
	}

	if options.Logger == nil {
		options.Logger = zap.NewNop()
	}

	h := &StaticAssetsHandler{
		options:  options,
		assetsFS: assetsFS,
	}

	indexHTML, err := h.loadAndEnrichIndexHTML(assetsFS.Open)
	if err != nil {
		return nil, err
	}

	options.Logger.Info("Using UI configuration", zap.String("path", options.UIConfigPath))
	watcher, err := fswatcher.New([]string{options.UIConfigPath}, h.reloadUIConfig, h.options.Logger)
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
	if configObject, err := loadUIConfig(sH.options.UIConfigPath); err != nil {
		return nil, err
	} else if configObject != nil {
		indexBytes = configObject.regexp.ReplaceAll(indexBytes, configObject.config)
	}
	// replace storage capabilities
	capabilitiesJSON, _ := json.Marshal(sH.options.StorageCapabilities)
	capabilitiesString := fmt.Sprintf("JAEGER_STORAGE_CAPABILITIES = %s;", string(capabilitiesJSON))
	indexBytes = compabilityPattern.ReplaceAll(indexBytes, []byte(capabilitiesString))
	// replace Jaeger version
	versionJSON, _ := json.Marshal(version.Get())
	versionString := fmt.Sprintf("JAEGER_VERSION = %s;", string(versionJSON))
	indexBytes = versionPattern.ReplaceAll(indexBytes, []byte(versionString))
	// replace base path
	if sH.options.BasePath == "" {
		sH.options.BasePath = "/"
	}
	if sH.options.BasePath != "/" {
		if !strings.HasPrefix(sH.options.BasePath, "/") || strings.HasSuffix(sH.options.BasePath, "/") {
			return nil, fmt.Errorf("invalid base path '%s'. Must start but not end with a slash '/', e.g. '/jaeger/ui'", sH.options.BasePath)
		}
		indexBytes = basePathPattern.ReplaceAll(indexBytes, []byte(fmt.Sprintf(`<base href="%s/"`, sH.options.BasePath)))
	}

	return indexBytes, nil
}

func (sH *StaticAssetsHandler) reloadUIConfig() {
	sH.options.Logger.Info("reloading UI config", zap.String("filename", sH.options.UIConfigPath))
	content, err := sH.loadAndEnrichIndexHTML(sH.assetsFS.Open)
	if err != nil {
		sH.options.Logger.Error("error while reloading the UI config", zap.Error(err))
	}
	sH.indexHTML.Store(content)
	sH.options.Logger.Info("reloaded UI config", zap.String("filename", sH.options.UIConfigPath))
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
		var c map[string]interface{}

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

// RegisterRoutes registers routes for this handler on the given router
func (sH *StaticAssetsHandler) RegisterRoutes(router *mux.Router) {
	fileServer := http.FileServer(sH.assetsFS)
	if sH.options.BasePath != "/" {
		fileServer = http.StripPrefix(sH.options.BasePath+"/", fileServer)
	}
	router.PathPrefix("/static/").Handler(sH.loggingHandler(fileServer))
	// index.html is served by notFound handler
	router.NotFoundHandler = sH.loggingHandler(http.HandlerFunc(sH.notFound))
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(sH.indexHTML.Load().([]byte))
}

func (sH *StaticAssetsHandler) Close() error {
	return sH.watcher.Close()
}
