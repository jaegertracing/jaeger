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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var (
	staticRootFiles = []string{"favicon.ico"}
	configPattern   = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
	basePathPattern = regexp.MustCompile(`<base href="/"`)
	basePathReplace = `<base href="%s/"`
	errBadBasePath  = "Invalid base path '%s'. Must start with / but not end with /, e.g. /jaeger/ui."
)

// RegisterStaticHandler adds handler for static assets to the router.
func RegisterStaticHandler(r *mux.Router, logger *zap.Logger, qOpts *QueryOptions) {
	staticHandler, err := NewStaticAssetsHandler(qOpts.StaticAssets, StaticAssetsHandlerOptions{
		BasePath:     qOpts.BasePath,
		UIConfigPath: qOpts.UIConfig,
	})
	if err != nil {
		logger.Panic("Could not create static assets handler", zap.Error(err))
	}
	if staticHandler != nil {
		staticHandler.RegisterRoutes(r)
	} else {
		logger.Info("Static handler is not registered")
	}
}

// StaticAssetsHandler handles static assets
type StaticAssetsHandler struct {
	options          StaticAssetsHandlerOptions
	staticAssetsRoot string
	indexHTML        []byte
}

// StaticAssetsHandlerOptions defines options for NewStaticAssetsHandler
type StaticAssetsHandlerOptions struct {
	BasePath     string
	UIConfigPath string
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string, options StaticAssetsHandlerOptions) (*StaticAssetsHandler, error) {
	if staticAssetsRoot == "" {
		return nil, nil
	}
	if !strings.HasSuffix(staticAssetsRoot, "/") {
		staticAssetsRoot = staticAssetsRoot + "/"
	}
	indexBytes, err := ioutil.ReadFile(staticAssetsRoot + "index.html")
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read UI static assets")
	}
	configString := "JAEGER_CONFIG = DEFAULT_CONFIG"
	if config, err := loadUIConfig(options.UIConfigPath); err != nil {
		return nil, err
	} else if config != nil {
		// TODO if we want to support other config formats like YAML, we need to normalize `config` to be
		// suitable for json.Marshal(). For example, YAML parser may return a map that has keys of type
		// interface{}, and json.Marshal() is unable to serialize it.
		bytes, _ := json.Marshal(config)
		configString = fmt.Sprintf("JAEGER_CONFIG = %v", string(bytes))
	}
	indexBytes = configPattern.ReplaceAll(indexBytes, []byte(configString+";"))
	if options.BasePath == "" {
		options.BasePath = "/"
	}
	if options.BasePath != "/" {
		if !strings.HasPrefix(options.BasePath, "/") || strings.HasSuffix(options.BasePath, "/") {
			return nil, fmt.Errorf(errBadBasePath, options.BasePath)
		}
		indexBytes = basePathPattern.ReplaceAll(indexBytes, []byte(fmt.Sprintf(basePathReplace, options.BasePath)))
	}
	return &StaticAssetsHandler{
		options:          options,
		staticAssetsRoot: staticAssetsRoot,
		indexHTML:        indexBytes,
	}, nil
}

func loadUIConfig(uiConfig string) (map[string]interface{}, error) {
	if uiConfig == "" {
		return nil, nil
	}
	ext := filepath.Ext(uiConfig)
	bytes, err := ioutil.ReadFile(uiConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot read UI config file %v", uiConfig)
	}

	var c map[string]interface{}
	var unmarshal func([]byte, interface{}) error

	switch strings.ToLower(ext) {
	case ".json":
		unmarshal = json.Unmarshal
	default:
		return nil, fmt.Errorf("Unrecognized UI config file format %v", uiConfig)
	}

	if err := unmarshal(bytes, &c); err != nil {
		return nil, errors.Wrapf(err, "Cannot parse UI config file %v", uiConfig)
	}
	return c, nil
}

// RegisterRoutes registers routes for this handler on the given router
func (sH *StaticAssetsHandler) RegisterRoutes(router *mux.Router) {
	// router.PathPrefix("/static").Handler(http.FileServer(http.Dir(sH.staticAssetsRoot)))
	router.PathPrefix("/static").Handler(sH.fileHandler())
	for _, file := range staticRootFiles {
		router.Path("/" + file).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, sH.staticAssetsRoot+file)
		})
	}
	router.NotFoundHandler = http.HandlerFunc(sH.notFound)
}

func (sH *StaticAssetsHandler) fileHandler() http.Handler {
	fs := http.FileServer(http.Dir(sH.staticAssetsRoot))
	base := sH.options.BasePath
	if base == "/" {
		return fs
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, base) {
			r.URL.Path = r.URL.Path[len(base):]
		}
		fs.ServeHTTP(w, r)
	})
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(sH.indexHTML)
}
