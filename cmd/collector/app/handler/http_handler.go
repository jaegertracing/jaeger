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

package handler

import (
	"fmt"
	"html"
	"io/ioutil"
	"mime"
	"net/http"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	tJaeger "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

const (
	// UnableToReadBodyErrFormat is an error message for invalid requests
	UnableToReadBodyErrFormat = "Unable to process request body: %v"
)

var (
	acceptedThriftFormats = map[string]struct{}{
		"application/x-thrift":                 {},
		"application/vnd.apache.thrift.binary": {},
	}
)

// APIHandler handles all HTTP calls to the collector
type APIHandler struct {
	jaegerBatchesHandler JaegerBatchesHandler
}

// NewAPIHandler returns a new APIHandler
func NewAPIHandler(
	jaegerBatchesHandler JaegerBatchesHandler,
) *APIHandler {
	return &APIHandler{
		jaegerBatchesHandler: jaegerBatchesHandler,
	}
}

// RegisterRoutes registers routes for this handler on the given router
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/traces", aH.SaveSpan).Methods(http.MethodPost)
}

// SaveSpan submits the span provided in the request body to the JaegerBatchesHandler
func (aH *APIHandler) SaveSpan(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, fmt.Sprintf(UnableToReadBodyErrFormat, err), http.StatusInternalServerError)
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))

	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot parse content type: %v", err), http.StatusBadRequest)
		return
	}

	if _, ok := acceptedThriftFormats[contentType]; !ok {
		http.Error(w, fmt.Sprintf("Unsupported content type: %v", html.EscapeString(contentType)), http.StatusBadRequest)
		return
	}

	tdes := thrift.NewTDeserializer()
	batch := &tJaeger.Batch{}
	if err = tdes.Read(batch, bodyBytes); err != nil {
		http.Error(w, fmt.Sprintf(UnableToReadBodyErrFormat, err), http.StatusBadRequest)
		return
	}
	batches := []*tJaeger.Batch{batch}
	opts := SubmitBatchOptions{InboundTransport: processor.HTTPTransport}
	if _, err = aH.jaegerBatchesHandler.SubmitBatches(batches, opts); err != nil {
		http.Error(w, fmt.Sprintf("Cannot submit Jaeger batch: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
