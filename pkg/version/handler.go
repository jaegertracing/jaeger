// Copyright (c) 2017 The Jaeger Authors.
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

package version

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// RegisterHandler registers version handler to /version
func RegisterHandler(mu *http.ServeMux, logger *zap.Logger) {
	info := Get()
	json, err := json.Marshal(info)
	if err != nil {
		logger.Fatal("Could not get Jaeger version", zap.Error(err))
	}
	mu.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write(json)
	})
}
