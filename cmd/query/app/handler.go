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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/model/adjuster"
	uiconv "github.com/uber/jaeger/model/converter/json"
	ui "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/multierror"
	"github.com/uber/jaeger/storage/dependencystore"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	traceIDParam  = "traceID"
	endTsParam    = "endTs"
	lookbackParam = "lookback"

	defaultDependencyLookbackDuration = time.Hour * 24
	defaultTraceQueryLookbackDuration = time.Hour * 24 * 2
)

// HTTPHandler handles http requests
type HTTPHandler interface {
	RegisterRoutes(router *mux.Router)
}

type structuredResponse struct {
	Data   interface{}       `json:"data"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Errors []structuredError `json:"errors"`
}

type structuredError struct {
	Code    int        `json:"code,omitempty"`
	Msg     string     `json:"msg"`
	TraceID ui.TraceID `json:"traceID,omitempty"`
}

// APIHandler implements the query service public API by registering routes at httpPrefix
type APIHandler struct {
	spanReader       spanstore.Reader
	dependencyReader dependencystore.Reader
	adjuster         adjuster.Adjuster
	logger           zap.Logger
	queryParser      queryParser
	httpPrefix       string
}

// NewAPIHandler returns an APIHandler
// TODO use Builder to allow this package to move to github while accepting custom components like HAProxyMerge()
func NewAPIHandler(spanReader spanstore.Reader, dependencyReader dependencystore.Reader, logger zap.Logger, httpPrefix string, adjusters []adjuster.Adjuster) *APIHandler {
	return &APIHandler{
		spanReader:       spanReader,
		dependencyReader: dependencyReader,
		logger:           logger,
		queryParser: queryParser{
			traceQueryLookbackDuration: defaultTraceQueryLookbackDuration,
		},
		adjuster:   adjuster.Sequence(adjusters...),
		httpPrefix: httpPrefix,
	}
}

// RegisterRoutes registers routes for this handler on the given router
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	// TODO why single-trace resource below accept POST method?
	router.HandleFunc(fmt.Sprintf("/%s/traces/{%s}", aH.httpPrefix, traceIDParam), aH.getTrace).Methods(http.MethodPost, http.MethodGet)
	router.HandleFunc(fmt.Sprintf(`/%s/traces`, aH.httpPrefix), aH.search).Methods(http.MethodGet)
	router.HandleFunc(fmt.Sprintf(`/%s/services`, aH.httpPrefix), aH.getServices).Methods(http.MethodGet)
	router.HandleFunc(fmt.Sprintf("/%s/services/operations", aH.httpPrefix), aH.getOperations).Methods(http.MethodGet)
	// TOOD - remove this when UI catches up
	router.HandleFunc(fmt.Sprintf("/api/services/{%s}/operations", serviceParam), aH.getOperationsLegacy).Methods(http.MethodGet)
	router.HandleFunc(fmt.Sprintf("/%s/dependencies", aH.httpPrefix), aH.dependencies).Methods(http.MethodGet)
}

func (aH *APIHandler) getServices(w http.ResponseWriter, r *http.Request) {
	services, err := aH.spanReader.GetServices()
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	structuredRes := structuredResponse{
		Data:  services,
		Total: len(services),
	}
	aH.writeJSON(w, &structuredRes)
}

func (aH *APIHandler) getOperationsLegacy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := vars[serviceParam] //given how getOperationsLegacy is used, this will never work never be empty
	operations, err := aH.spanReader.GetOperations(service)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	structuredRes := structuredResponse{
		Data:  operations,
		Total: len(operations),
	}
	aH.writeJSON(w, &structuredRes)
}

func (aH *APIHandler) getOperations(w http.ResponseWriter, r *http.Request) {
	service := r.FormValue(serviceParam)
	if service == "" {
		if aH.handleError(w, ErrServiceParameterRequired, http.StatusBadRequest) {
			return
		}
	}
	operations, err := aH.spanReader.GetOperations(service)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	structuredRes := structuredResponse{
		Data:  operations,
		Total: len(operations),
	}
	aH.writeJSON(w, &structuredRes)
}

func (aH *APIHandler) search(w http.ResponseWriter, r *http.Request) {
	tQuery, err := aH.queryParser.parse(r)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}
	tracesFromStorage, err := aH.spanReader.FindTraces(tQuery)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	// TODO: parallelize transformations
	uiTraces := make([]*ui.Trace, len(tracesFromStorage))
	var uiErrors []structuredError
	for i, v := range tracesFromStorage {
		uiTrace, uiErr := aH.convertModelToUI(v)
		if uiErr != nil {
			uiErrors = append(uiErrors, *uiErr)
		}
		uiTraces[i] = uiTrace
	}

	structuredRes := structuredResponse{
		Data:   uiTraces,
		Errors: uiErrors,
	}
	aH.writeJSON(w, &structuredRes)
}

func (aH *APIHandler) dependencies(w http.ResponseWriter, r *http.Request) {
	endTsMillis, err := strconv.ParseInt(r.FormValue(endTsParam), 10, 64)
	if aH.handleError(w, errors.Wrapf(err, "Unable to parse %s", endTimeParam), http.StatusBadRequest) {
		return
	}
	var lookback time.Duration
	if formValue := r.FormValue(lookbackParam); len(formValue) > 0 {
		lookback, err = time.ParseDuration(formValue + "ms")
		if aH.handleError(w, errors.Wrapf(err, "Unable to parse %s", lookbackParam), http.StatusBadRequest) {
			return
		}
	}
	service := r.FormValue(serviceParam)

	if lookback == 0 {
		lookback = defaultDependencyLookbackDuration
	}
	endTs := time.Unix(0, 0).Add(time.Duration(endTsMillis) * time.Millisecond)

	dependencies, err := aH.dependencyReader.GetDependencies(endTs, lookback)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	filteredDependencies := aH.filterDependenciesByService(dependencies, service)
	structuredRes := structuredResponse{
		Data: aH.deduplicateDependencies(filteredDependencies),
	}
	aH.writeJSON(w, &structuredRes)
}

func (aH *APIHandler) convertModelToUI(traceFromStorage *model.Trace) (*ui.Trace, *structuredError) {
	var errors []error
	trace, err := aH.adjuster.Adjust(traceFromStorage)
	if err != nil {
		errors = append(errors, err)
	}
	uiTrace := uiconv.FromDomain(trace)
	var uiError *structuredError
	if err := multierror.Wrap(errors); err != nil {
		uiError = &structuredError{
			Msg:     err.Error(),
			TraceID: uiTrace.TraceID,
		}
	}
	return uiTrace, uiError
}

func (aH *APIHandler) deduplicateDependencies(dependencies []model.DependencyLink) []ui.DependencyLink {
	type Key struct {
		parent string
		child  string
	}
	links := make(map[Key]uint64)

	for _, l := range dependencies {
		links[Key{l.Parent, l.Child}] += l.CallCount
	}

	result := make([]ui.DependencyLink, 0, len(links))
	for k, v := range links {
		result = append(result, ui.DependencyLink{Parent: k.parent, Child: k.child, CallCount: v})
	}

	return result
}

func (aH *APIHandler) filterDependenciesByService(
	dependencies []model.DependencyLink,
	service string,
) []model.DependencyLink {
	if len(service) == 0 {
		return dependencies
	}

	var filteredDependencies []model.DependencyLink
	for _, dependency := range dependencies {
		if dependency.Parent == service || dependency.Child == service {
			filteredDependencies = append(filteredDependencies, dependency)
		}
	}
	return filteredDependencies
}

func (aH *APIHandler) getTrace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	traceIDVar := vars[traceIDParam]
	traceID, err := model.TraceIDFromString(traceIDVar)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}

	zTrace, err := aH.spanReader.GetTrace(traceID)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	var uiErrors []structuredError
	uiTrace, uiErr := aH.convertModelToUI(zTrace)
	if uiErr != nil {
		uiErrors = append(uiErrors, *uiErr)
	}

	structuredRes := structuredResponse{
		Data: []*ui.Trace{
			uiTrace,
		},
		Errors: uiErrors,
	}
	aH.writeJSON(w, &structuredRes)
}

func (aH *APIHandler) handleError(w http.ResponseWriter, err error, statusCode int) bool {
	if err == nil {
		return false
	}
	structuredResp := structuredResponse{
		Errors: []structuredError{
			{
				Code: statusCode,
				Msg:  err.Error(),
			},
		},
	}
	resp, _ := json.Marshal(&structuredResp)
	http.Error(w, string(resp), statusCode)
	return true
}

func (aH *APIHandler) writeJSON(w http.ResponseWriter, response *structuredResponse) {
	resp, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}
