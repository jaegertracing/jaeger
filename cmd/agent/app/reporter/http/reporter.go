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

package http

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	jaegerBatches = "jaeger"
	zipkinBatches = "zipkin"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

// Reporter forwards received spans to central collector tier over plain HTTP.
type Reporter struct {
	scheme             string   // http or https
	collectorHostPorts []string // only the first value is used

	authToken string
	username  string
	password  string

	batchesMetrics map[string]reporter.BatchMetrics
	logger         *zap.Logger
}

// New creates new HTTP-based Reporter.
func New(
	scheme string,
	collectorHostPorts []string,

	authToken string,
	username string,
	password string,

	mFactory metrics.Factory,
	zlogger *zap.Logger,
) (*Reporter, error) {
	batchesMetrics := map[string]reporter.BatchMetrics{}
	tcReporterNS := mFactory.Namespace("http-reporter", nil)
	for _, s := range []string{zipkinBatches, jaegerBatches} {
		nsByType := tcReporterNS.Namespace(s, nil)
		bm := reporter.BatchMetrics{}
		metrics.Init(&bm, nsByType, nil)
		batchesMetrics[s] = bm
	}

	if scheme != "http" && scheme != "https" {
		zlogger.Warn(`unknown scheme for HTTP communication with Collector, reverting to "http".`, zap.String("scheme", scheme))
		scheme = "http"
	}

	if len(collectorHostPorts) > 1 {
		zlogger.Info(`more than one "collectorHostPorts" was specified. Only the first one will be used.`)
	}

	if len(collectorHostPorts) == 0 {
		return nil, errors.New(`no "collectorHostPorts" specified`)
	}

	// be strict with what you send, be lenient with what you receive:
	for i, c := range collectorHostPorts {
		// we are seeing a host with protocol, that might start with http or https
		if c[0:4] == "http" {
			if u, err := url.Parse(c); err == nil {
				// if we can parse this url without errors, we just use the host from this url
				collectorHostPorts[i] = u.Host
				scheme = u.Scheme

				// and let the user know that we are changing the value they specied!
				// they might have specified a path and we are ignoring it
				zlogger.Info(`the "collectorHostPorts" was expected to have only hostname:port.`, zap.String("collectorHostPorts", c))
			}
		}
	}

	return &Reporter{
		scheme:             scheme,
		collectorHostPorts: collectorHostPorts,

		authToken: authToken,
		username:  username,
		password:  password,

		logger:         zlogger,
		batchesMetrics: batchesMetrics,
	}, nil
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *Reporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	submissionFunc := func() error {
		t := thrift.NewTMemoryBuffer()
		p := thrift.NewTBinaryProtocolTransport(t)
		p.WriteListBegin(thrift.STRUCT, len(spans))
		for _, s := range spans {
			s.Write(p)
		}
		p.WriteListEnd()
		body := t.Buffer.Bytes()

		url := r.Endpoint() + `/api/v1/spans`

		// this can't fail: both the HTTP and the URL method will always be valid
		// the only part of the URL that can be invalid is the hostname, but we'll fail
		// early on if it's invalid
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))

		req.Header.Add("Content-Type", "application/x-thrift")
		r.addAuthHeaders(req)

		res, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode > 299 { // we are assuming that redirects are bad
			return fmt.Errorf("failed to submit batch: %s", res.Status)
		}

		return nil
	}

	return r.submitAndReport(
		submissionFunc,
		"Could not submit zipkin batch",
		int64(len(spans)),
		r.batchesMetrics[zipkinBatches],
	)
}

// EmitBatch implements EmitBatch() of Reporter
func (r *Reporter) EmitBatch(batch *jaeger.Batch) error {
	submissionFunc := func() error {
		tser := thrift.NewTSerializer()
		body, _ := tser.Write(batch)

		url := r.Endpoint() + `/api/traces?format=jaeger.thrift`

		// this can't fail: both the HTTP and the URL method will always be valid
		// the only part of the URL that can be invalid is the hostname, but we'll fail
		// early on if it's invalid
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer([]byte(body)))

		r.addAuthHeaders(req)

		res, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode > 299 { // we are assuming that redirects are bad
			return fmt.Errorf("failed to submit batch: %s", res.Status)
		}

		return nil
	}

	return r.submitAndReport(
		submissionFunc,
		"Could not submit jaeger batch",
		int64(len(batch.Spans)),
		r.batchesMetrics[jaegerBatches],
	)
}

// Endpoint returns the endpoint used when communicating with the remote collector
func (r *Reporter) Endpoint() string {
	// TODO: do we want to do client-side load balancing? Or use a retry logic?
	// For now, we just get the first one!
	return fmt.Sprintf("%s://%s", r.scheme, r.collectorHostPorts[0])
}

func (r *Reporter) submitAndReport(submissionFunc func() error, errMsg string, size int64, batchMetrics reporter.BatchMetrics) error {
	if err := submissionFunc(); err != nil {
		batchMetrics.BatchesFailures.Inc(1)
		batchMetrics.SpansFailures.Inc(size)
		r.logger.Error(errMsg, zap.Error(err))
		return err
	}
	batchMetrics.BatchSize.Update(size)
	batchMetrics.BatchesSubmitted.Inc(1)
	batchMetrics.SpansSubmitted.Inc(size)
	return nil
}

func (r *Reporter) addAuthHeaders(req *http.Request) {
	if r.username != "" && r.password != "" {
		req.SetBasicAuth(r.username, r.password)
	} else if r.authToken != "" {
		req.Header.Add("Authorization", "Bearer "+r.authToken)
	}
}
