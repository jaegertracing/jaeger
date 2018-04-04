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
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	uiconv "github.com/jaegertracing/jaeger/model/converter/json"
	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	traceIDParam  = "traceID"
	endTsParam    = "endTs"
	lookbackParam = "lookback"

	defaultDependencyLookbackDuration = time.Hour * 24
	defaultTraceQueryLookbackDuration = time.Hour * 24 * 2
	defaultAPIPrefix                  = "api"
)

var (
	errNoArchiveSpanStorage = errors.New("archive span storage was not configured")
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

// NewRouter creates and configures a Gorilla Router.
func NewRouter() *mux.Router {
	return mux.NewRouter().UseEncodedPath()
}

// APIHandler implements the query service public API by registering routes at httpPrefix
type APIHandler struct {
	spanReader        spanstore.Reader
	archiveSpanReader spanstore.Reader
	archiveSpanWriter spanstore.Writer
	dependencyReader  dependencystore.Reader
	adjuster          adjuster.Adjuster
	logger            *zap.Logger
	queryParser       queryParser
	basePath          string
	apiPrefix         string
	tracer            opentracing.Tracer
}

// NewAPIHandler returns an APIHandler
func NewAPIHandler(spanReader spanstore.Reader, dependencyReader dependencystore.Reader, options ...HandlerOption) *APIHandler {
	aH := &APIHandler{
		spanReader:       spanReader,
		dependencyReader: dependencyReader,
		queryParser: queryParser{
			traceQueryLookbackDuration: defaultTraceQueryLookbackDuration,
			timeNow:                    time.Now,
		},
	}

	for _, option := range options {
		option(aH)
	}
	if aH.apiPrefix == "" {
		aH.apiPrefix = defaultAPIPrefix
	}
	if aH.adjuster == nil {
		aH.adjuster = adjuster.Sequence(StandardAdjusters...)
	}
	if aH.logger == nil {
		aH.logger = zap.NewNop()
	}
	if aH.tracer == nil {
		aH.tracer = opentracing.NoopTracer{}
	}
	return aH
}

// RegisterRoutes registers routes for this handler on the given router
func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	aH.handleFunc(router, aH.getTrace, "/traces/{%s}", traceIDParam).Methods(http.MethodGet)
	aH.handleFunc(router, aH.archiveTrace, "/archive/{%s}", traceIDParam).Methods(http.MethodPost)
	aH.handleFunc(router, aH.search, "/traces").Methods(http.MethodGet)
	aH.handleFunc(router, aH.getServices, "/services").Methods(http.MethodGet)
	// TODO change the UI to use this endpoint. Requires ?service= parameter.
	aH.handleFunc(router, aH.getOperations, "/operations").Methods(http.MethodGet)
	// TODO - remove this when UI catches up
	aH.handleFunc(router, aH.getOperationsLegacy, "/services/{%s}/operations", serviceParam).Methods(http.MethodGet)
	aH.handleFunc(router, aH.dependencies, "/dependencies").Methods(http.MethodGet)
}

func (aH *APIHandler) handleFunc(
	router *mux.Router,
	f func(http.ResponseWriter, *http.Request),
	route string,
	args ...interface{},
) *mux.Route {
	route = aH.route(route, args...)
	traceMiddleware := nethttp.Middleware(
		aH.tracer,
		http.HandlerFunc(f),
		nethttp.OperationNameFunc(func(r *http.Request) string {
			return route
		}))
	return router.HandleFunc(route, traceMiddleware.ServeHTTP)
}

func (aH *APIHandler) route(route string, args ...interface{}) string {
	args = append([]interface{}{aH.apiPrefix}, args...)
	return fmt.Sprintf("/%s"+route, args...)
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
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) getOperationsLegacy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	// given how getOperationsLegacy is bound to URL route, serviceParam cannot be empty
	service, _ := url.QueryUnescape(vars[serviceParam])
	operations, err := aH.spanReader.GetOperations(service)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	structuredRes := structuredResponse{
		Data:  operations,
		Total: len(operations),
	}
	aH.writeJSON(w, r, &structuredRes)
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
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) search(w http.ResponseWriter, r *http.Request) {
	tQuery, err := aH.queryParser.parse(r)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return
	}

	var uiErrors []structuredError
	var tracesFromStorage []*model.Trace
	if len(tQuery.traceIDs) > 0 {
		tracesFromStorage, uiErrors, err = aH.tracesByIDs(tQuery.traceIDs)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
	} else {
		tracesFromStorage, err = aH.spanReader.FindTraces(&tQuery.TraceQueryParameters)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
	}

	uiTraces := make([]*ui.Trace, len(tracesFromStorage))
	for i, v := range tracesFromStorage {
		uiTrace, uiErr := aH.convertModelToUI(v, true)
		if uiErr != nil {
			uiErrors = append(uiErrors, *uiErr)
		}
		uiTraces[i] = uiTrace
	}

	structuredRes := structuredResponse{
		Data:   uiTraces,
		Errors: uiErrors,
	}
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) tracesByIDs(traceIDs []model.TraceID) ([]*model.Trace, []structuredError, error) {
	var errors []structuredError
	retMe := make([]*model.Trace, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		if trace, err := aH.spanReader.GetTrace(traceID); err != nil {
			if err != spanstore.ErrTraceNotFound {
				return nil, nil, err
			}
			errors = append(errors, structuredError{
				Msg:     err.Error(),
				TraceID: ui.TraceID(traceID.String()),
			})
		} else {
			retMe = append(retMe, trace)
		}
	}
	return retMe, errors, nil
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
	aH.writeJSON(w, r, &structuredRes)
}

func (aH *APIHandler) convertModelToUI(trace *model.Trace, adjust bool) (*ui.Trace, *structuredError) {
	var errors []error
	if adjust {
		var err error
		trace, err = aH.adjuster.Adjust(trace)
		if err != nil {
			errors = append(errors, err)
		}
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

// Parses trace ID from URL like /traces/{trace-id}
func (aH *APIHandler) parseTraceID(w http.ResponseWriter, r *http.Request) (model.TraceID, bool) {
	vars := mux.Vars(r)
	traceIDVar := vars[traceIDParam]
	traceID, err := model.TraceIDFromString(traceIDVar)
	if aH.handleError(w, err, http.StatusBadRequest) {
		return traceID, false
	}
	return traceID, true
}

// getTrace implements the REST API /traces/{trace-id}
func (aH *APIHandler) getTrace(w http.ResponseWriter, r *http.Request) {
	aH.getTraceFromReaders(w, r, aH.spanReader, aH.archiveSpanReader)
}

// getTraceFromReader parses trace ID from the path, loads the trace from specified Reader,
// formats it in the UI JSON format, and responds to the client.
func (aH *APIHandler) getTraceFromReaders(
	w http.ResponseWriter,
	r *http.Request,
	reader spanstore.Reader,
	backupReader spanstore.Reader,
) {
	aH.withTraceFromReader(w, r, reader, backupReader, func(trace *model.Trace) {
		var uiErrors []structuredError
		uiTrace, uiErr := aH.convertModelToUI(trace, shouldAdjust(r))
		if uiErr != nil {
			uiErrors = append(uiErrors, *uiErr)
		}

		structuredRes := structuredResponse{
			Data: []*ui.Trace{
				uiTrace,
			},
			Errors: uiErrors,
		}
		aH.writeJSON(w, r, &structuredRes)
	})
}

func shouldAdjust(r *http.Request) bool {
	raw := r.FormValue("raw")
	isRaw, _ := strconv.ParseBool(raw)
	return !isRaw
}

// withTraceFromReader tries to load a trace from Reader and if successful
// execute process() function passing it that trace.
func (aH *APIHandler) withTraceFromReader(
	w http.ResponseWriter,
	r *http.Request,
	reader spanstore.Reader,
	backupReader spanstore.Reader,
	process func(trace *model.Trace),
) {
	traceID, ok := aH.parseTraceID(w, r)
	if !ok {
		return
	}
	trace, err := reader.GetTrace(traceID)
	if err == spanstore.ErrTraceNotFound {
		if backupReader == nil {
			aH.handleError(w, err, http.StatusNotFound)
			return
		}
		trace, err = backupReader.GetTrace(traceID)
		if err == spanstore.ErrTraceNotFound {
			aH.handleError(w, err, http.StatusNotFound)
			return
		}
	}
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}
	process(trace)
}

// archiveTrace implements the REST API POST:/archive/{trace-id}.
// It reads the trace from the main Reader and saves it to archive Writer.
func (aH *APIHandler) archiveTrace(w http.ResponseWriter, r *http.Request) {
	if aH.archiveSpanWriter == nil {
		aH.handleError(w, errNoArchiveSpanStorage, http.StatusInternalServerError)
		return
	}
	aH.withTraceFromReader(w, r, aH.spanReader, nil, func(trace *model.Trace) {
		var writeErrors []error
		for _, span := range trace.Spans {
			err := aH.archiveSpanWriter.WriteSpan(span)
			if err != nil {
				writeErrors = append(writeErrors, err)
			}
		}
		err := multierror.Wrap(writeErrors)
		if aH.handleError(w, err, http.StatusInternalServerError) {
			return
		}
		structuredRes := structuredResponse{
			Data:   []string{}, // doens't matter, just want an empty array
			Errors: []structuredError{},
		}
		aH.writeJSON(w, r, &structuredRes)
	})
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

func (aH *APIHandler) writeJSON(w http.ResponseWriter, r *http.Request, response interface{}) {
	marshall := json.Marshal
	if prettyPrint := r.FormValue(prettyPrintParam); prettyPrint != "" && prettyPrint != "false" {
		marshall = func(v interface{}) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
	}
	resp, _ := marshall(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}
