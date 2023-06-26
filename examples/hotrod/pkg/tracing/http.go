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

package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"

	"go.opentelemetry.io/otel/trace"
)

type contextKey int

const (
	keyTracer contextKey = iota
)

// HTTPClient wraps an http.Client with tracing instrumentation.
type HTTPClient struct {
	Tracer trace.TracerProvider
	Client *http.Client
}

// Tracer holds tracing details for one HTTP request.
type Tracer struct {
	tp   trace.TracerProvider
	root trace.Span
	sp   trace.Span
	opts *clientOptions
}

type clientOptions struct {
	operationName            string
	componentName            string
	urlTagFunc               func(u *url.URL) string
	disableClientTrace       bool
	disableInjectSpanContext bool
	spanObserver             func(span trace.Span, r *http.Request)
}

// ClientOption contols the behavior of TraceRequest.
type ClientOption func(*clientOptions)

// GetJSON executes HTTP GET against specified url and tried to parse
// the response into out object.
func (c *HTTPClient) GetJSON(ctx context.Context, endpoint string, url string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req, ht := c.TraceRequest(c.Tracer, req, c.OperationName("HTTP GET: "+endpoint))
	defer ht.Finish()

	res, err := c.Client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		return errors.New(string(body))
	}
	decoder := json.NewDecoder(res.Body)
	return decoder.Decode(out)
}

func (c *HTTPClient) TraceRequest(tp trace.TracerProvider, req *http.Request, options ...ClientOption) (*http.Request, *Tracer) {
	opts := &clientOptions{
		urlTagFunc: func(u *url.URL) string {
			return u.String()
		},
		spanObserver: func(_ trace.Span, _ *http.Request) {},
	}
	for _, opt := range options {
		opt(opts)
	}
	ht := &Tracer{tp: tp, opts: opts}
	ctx := req.Context()
	if !opts.disableClientTrace {
		ctx = httptrace.WithClientTrace(ctx, ht.clientTrace())
	}
	req = req.WithContext(context.WithValue(ctx, keyTracer, ht))
	return req, ht
}

// OperationName returns a ClientOption that sets the operation
// name for the client-side span.
func (c *HTTPClient) OperationName(operationName string) ClientOption {
	return func(options *clientOptions) {
		options.operationName = operationName
	}
}

// Finish finishes the span of the traced request.
func (h *Tracer) Finish() {
	if h.root != nil {
		h.root.End()
	}
}

func (h *Tracer) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{}
}
