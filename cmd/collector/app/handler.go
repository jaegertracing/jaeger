// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
)

// APIHandler handles all HTTP calls to the collector
type APIHandler struct {
	jaegerBatchesHandler JaegerBatchesHandler
}

// NewAPIHandler returns a new APIHandler
func NewAPIHandler(jaegerBatchesHandler JaegerBatchesHandler) *APIHandler {
	return &APIHandler{
		jaegerBatchesHandler: jaegerBatchesHandler,
	}
}

// RegisterRoutes registers routes for this handler on the given router
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/traces", aH.saveSpan).Methods(http.MethodPost)
}

func (aH *APIHandler) saveSpan(w http.ResponseWriter, r *http.Request) {
	format := r.FormValue(formatParam)
	switch strings.ToLower(format) {
	case "jaeger.thrift":
		bodyBytes, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to read from body due to error: %v", err), http.StatusBadRequest)
			return
		}

		tdes := thrift.NewTDeserializer()

		var req tJaeger.CollectorSubmitBatchesArgs

		err = tdes.Read(&req, bodyBytes)
		if err != nil {
			http.Error(w, fmt.Sprintf("Cannot deserialize body due to error: %v", err), http.StatusBadRequest)
			return
		}

		ctx, cancel := tchanThrift.NewContext(time.Minute)
		defer cancel()
		_, err = aH.jaegerBatchesHandler.SubmitBatches(ctx, req.Batches)
		if err != nil {
			http.Error(w, fmt.Sprintf("Cannot submit Jaeger batch due to error: %v", err), http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	default:
		http.Error(w, fmt.Sprintf("Unsupported format type: %v", format), http.StatusBadRequest)
	}
}
