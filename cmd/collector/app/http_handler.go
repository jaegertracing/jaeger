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
	"strings"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"
	tchanThrift "github.com/uber/tchannel-go/thrift"

	tJaeger "github.com/uber/jaeger/thrift-gen/jaeger"
)

const (
	formatParam = "format"
	// UnableToReadBodyErrFormat is an error message for invalid requests
	UnableToReadBodyErrFormat = "Unable to process request body: %v"
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
	router.HandleFunc("/api/traces", aH.saveSpan).Methods(http.MethodPost)
}

func (aH *APIHandler) saveSpan(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, fmt.Sprintf(UnableToReadBodyErrFormat, err), http.StatusInternalServerError)
		return
	}

	format := r.FormValue(formatParam)
	switch strings.ToLower(format) {
	case "jaeger.thrift":
		tdes := thrift.NewTDeserializer()
		// (NB): We decided to use this struct instead of straight batches to be as consistent with tchannel intake as possible.
		batch := &tJaeger.Batch{}
		if err = tdes.Read(batch, bodyBytes); err != nil {
			http.Error(w, fmt.Sprintf(UnableToReadBodyErrFormat, err), http.StatusBadRequest)
			return
		}
		ctx, cancel := tchanThrift.NewContext(time.Minute)
		defer cancel()
		batches := []*tJaeger.Batch{batch}
		if _, err = aH.jaegerBatchesHandler.SubmitBatches(ctx, batches); err != nil {
			http.Error(w, fmt.Sprintf("Cannot submit Jaeger batch: %v", err), http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, fmt.Sprintf("Unsupported format type: %v", format), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
