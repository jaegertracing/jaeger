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

package zipkin

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"
	tchanThrift "github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/cmd/collector/app"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

// APIHandler handles all HTTP calls to the collector
type APIHandler struct {
	zipkinSpansHandler app.ZipkinSpansHandler
}

// NewAPIHandler returns a new APIHandler
func NewAPIHandler(
	zipkinSpansHandler app.ZipkinSpansHandler,
) *APIHandler {
	return &APIHandler{
		zipkinSpansHandler: zipkinSpansHandler,
	}
}

// RegisterRoutes registers Zipkin routes
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/v1/spans", aH.saveSpans).Methods(http.MethodPost)
}

func (aH *APIHandler) saveSpans(w http.ResponseWriter, r *http.Request) {
	bRead := r.Body
	defer r.Body.Close()

	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf(app.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
			return
		}
		defer gz.Close()
		bRead = gz
	}

	bodyBytes, err := ioutil.ReadAll(bRead)
	if err != nil {
		http.Error(w, fmt.Sprintf(app.UnableToReadBodyErrFormat, err), http.StatusInternalServerError)
		return
	}

	contentType := r.Header.Get("Content-Type")
	var tSpans []*zipkincore.Span
	if contentType == "application/x-thrift" {
		tSpans, err = deserializeThrift(bodyBytes)
		if err != nil {
			http.Error(w, fmt.Sprintf(app.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
			return
		}
	} else if contentType == "application/json" {
		tSpans, err = deserializeJSON(bodyBytes)
		if err != nil {
			http.Error(w, fmt.Sprintf(app.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "Not supported Content-Type", http.StatusBadRequest)
	}

	if len(tSpans) > 0 {
		ctx, _ := tchanThrift.NewContext(time.Minute)
		if _, err = aH.zipkinSpansHandler.SubmitZipkinBatch(ctx, tSpans); err != nil {
			http.Error(w, fmt.Sprintf("Cannot submit Zipkin batch: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

func deserializeThrift(b []byte) ([]*zipkincore.Span, error) {
	buffer := thrift.NewTMemoryBuffer()
	buffer.Write(b)

	transport := thrift.NewTBinaryProtocolTransport(buffer)
	_, size, err := transport.ReadListBegin() // Ignore the returned element type
	if err != nil {
		return nil, err
	}

	// We don't depend on the size returned by ReadListBegin to preallocate the array because it
	// sometimes returns a nil error on bad input and provides an unreasonably large int for size
	var spans []*zipkincore.Span
	for i := 0; i < size; i++ {
		zs := &zipkincore.Span{}
		if err = zs.Read(transport); err != nil {
			return nil, err
		}
		spans = append(spans, zs)
	}

	return spans, nil
}
