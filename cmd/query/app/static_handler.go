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
	"gopkg.in/yaml.v2"
)

const (
	defaultStaticAssetsRoot = "jaeger-ui-build/build/"
)

var (
	staticRootFiles = []string{"favicon.ico"}
	configPattern   = regexp.MustCompile("JAEGER_CONFIG *= *DEFAULT_CONFIG;")
)

// StaticAssetsHandler handles static assets
type StaticAssetsHandler struct {
	staticAssetsRoot string
	indexHTML        []byte
}

// NewStaticAssetsHandler returns a StaticAssetsHandler
func NewStaticAssetsHandler(staticAssetsRoot string, uiConfig string) (*StaticAssetsHandler, error) {
	if staticAssetsRoot == "" {
		staticAssetsRoot = defaultStaticAssetsRoot
	}
	if !strings.HasSuffix(staticAssetsRoot, "/") {
		staticAssetsRoot = staticAssetsRoot + "/"
	}
	indexBytes, err := ioutil.ReadFile(staticAssetsRoot + "index.html")
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read UI static assets")
	}
	configString := "JAEGER_CONFIG = DEFAULT_CONFIG;"
	if config, err := loadUIConfig(uiConfig); err != nil {
		return nil, err
	} else if config != nil {
		bytes, _ := json.Marshal(config)
		configString = fmt.Sprintf("JAEGER_CONFIG = %v;", string(bytes))
	}
	return &StaticAssetsHandler{
		staticAssetsRoot: staticAssetsRoot,
		indexHTML:        configPattern.ReplaceAll(indexBytes, []byte(configString)),
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
	case ".yaml", ".yml":
		unmarshal = yaml.Unmarshal
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
	router.PathPrefix("/static").Handler(http.FileServer(http.Dir(sH.staticAssetsRoot)))
	for _, file := range staticRootFiles {
		router.Path("/" + file).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, sH.staticAssetsRoot+file)
		})
	}
	router.NotFoundHandler = http.HandlerFunc(sH.notFound)
}

func (sH *StaticAssetsHandler) notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(sH.indexHTML)
}
