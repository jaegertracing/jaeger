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

package zipkin

import (
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	zipkinProto "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/swagger-gen/models"
	"github.com/jaegertracing/jaeger/swagger-gen/restapi"
	"github.com/jaegertracing/jaeger/swagger-gen/restapi/operations"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// APIHandler handles all HTTP calls to the collector
type APIHandler struct {
	zipkinSpansHandler handler.ZipkinSpansHandler
	zipkinV2Formats    strfmt.Registry
}

// NewAPIHandler returns a new APIHandler
func NewAPIHandler(
	zipkinSpansHandler handler.ZipkinSpansHandler,
) *APIHandler {
	swaggerSpec, _ := loads.Analyzed(restapi.SwaggerJSON, "")
	return &APIHandler{
		zipkinSpansHandler: zipkinSpansHandler,
		zipkinV2Formats:    operations.NewZipkinAPI(swaggerSpec).Formats(),
	}
}

// RegisterRoutes registers Zipkin routes
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/v1/spans", aH.saveSpans).Methods(http.MethodPost)
	router.HandleFunc("/api/v2/spans", aH.saveSpansV2).Methods(http.MethodPost)
}

func (aH *APIHandler) saveSpans(w http.ResponseWriter, r *http.Request) {
	bRead := r.Body
	defer r.Body.Close()
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gunzip(bRead)
		if err != nil {
			http.Error(w, fmt.Sprintf(handler.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
			return
		}
		defer gz.Close()
		bRead = gz
	}

	bodyBytes, err := ioutil.ReadAll(bRead)
	if err != nil {
		http.Error(w, fmt.Sprintf(handler.UnableToReadBodyErrFormat, err), http.StatusInternalServerError)
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))

	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot parse Content-Type: %v", err), http.StatusBadRequest)
		return
	}

	var tSpans []*zipkincore.Span
	switch contentType {
	case "application/x-thrift":
		tSpans, err = zipkin.DeserializeThrift(bodyBytes)
	case "application/json":
		tSpans, err = DeserializeJSON(bodyBytes)
	default:
		http.Error(w, "Unsupported Content-Type", http.StatusBadRequest)
		return
	}
	if err != nil {
		safeErr := html.EscapeString(err.Error())
		http.Error(w, fmt.Sprintf(handler.UnableToReadBodyErrFormat, safeErr), http.StatusBadRequest)
		return
	}

	if err := aH.saveThriftSpans(tSpans); err != nil {
		http.Error(w, fmt.Sprintf("Cannot submit Zipkin batch: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (aH *APIHandler) saveSpansV2(w http.ResponseWriter, r *http.Request) {
	bRead := r.Body
	defer r.Body.Close()
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gunzip(bRead)
		if err != nil {
			http.Error(w, fmt.Sprintf(handler.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
			return
		}
		defer gz.Close()
		bRead = gz
	}

	bodyBytes, err := ioutil.ReadAll(bRead)
	if err != nil {
		http.Error(w, fmt.Sprintf(handler.UnableToReadBodyErrFormat, err), http.StatusInternalServerError)
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))

	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot parse Content-Type: %v", err), http.StatusBadRequest)
		return
	}

	var tSpans []*zipkincore.Span
	switch contentType {
	case "application/json":
		tSpans, err = jsonToThriftSpansV2(bodyBytes, aH.zipkinV2Formats)
	case "application/x-protobuf":
		tSpans, err = protoToThriftSpansV2(bodyBytes)
	default:
		http.Error(w, "Unsupported Content-Type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(handler.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
		return
	}

	if err = aH.saveThriftSpans(tSpans); err != nil {
		http.Error(w, fmt.Sprintf("Cannot submit Zipkin batch: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(operations.PostSpansAcceptedCode)
}

func jsonToThriftSpansV2(bodyBytes []byte, zipkinV2Formats strfmt.Registry) ([]*zipkincore.Span, error) {
	var spans models.ListOfSpans
	if err := swag.ReadJSON(bodyBytes, &spans); err != nil {
		return nil, err
	}
	if err := spans.Validate(zipkinV2Formats); err != nil {
		return nil, err
	}

	tSpans, err := spansV2ToThrift(spans)
	if err != nil {
		return nil, err
	}
	return tSpans, nil
}

func protoToThriftSpansV2(bodyBytes []byte) ([]*zipkincore.Span, error) {
	var spans zipkinProto.ListOfSpans
	if err := proto.Unmarshal(bodyBytes, &spans); err != nil {
		return nil, err
	}

	tSpans, err := protoSpansV2ToThrift(&spans)
	if err != nil {
		return nil, err
	}
	return tSpans, nil
}

func gunzip(r io.ReadCloser) (*gzip.Reader, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return gz, nil
}

func (aH *APIHandler) saveThriftSpans(tSpans []*zipkincore.Span) error {
	if len(tSpans) > 0 {
		opts := handler.SubmitBatchOptions{InboundTransport: processor.HTTPTransport}
		if _, err := aH.zipkinSpansHandler.SubmitZipkinBatch(tSpans, opts); err != nil {
			return err
		}
	}
	return nil
}
