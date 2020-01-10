// Copyright (c) 2020 The Jaeger Authors.
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

package reporter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

type clientMetricsTest struct {
	mr   *testutils.InMemoryReporter
	r    *ClientMetricsReporter
	logs *observer.ObservedLogs
}

func testClientMetrics(fn func(tr *clientMetricsTest)) {
	r1 := testutils.NewInMemoryReporter()
	zapCore, logs := observer.New(zap.DebugLevel)
	logger := zap.New(zapCore)
	r := WrapWithClientMetrics(ClientMetricsReporterParams{
		Reporter:       r1,
		Logger:         logger,
		MetricsFactory: metrics.NullFactory,
	})
	// don't close reporter

	tr := &clientMetricsTest{
		mr:   r1,
		r:    r,
		logs: logs,
	}
	fn(tr)
}

func TestClientMetricsReporterZipkin(t *testing.T) {
	testClientMetrics(func(tr *clientMetricsTest) {
		defer tr.r.Close()

		assert.NoError(t, tr.r.EmitZipkinBatch([]*zipkincore.Span{{}}))
		assert.Len(t, tr.mr.ZipkinSpans(), 1)
	})
}

func TestClientMetricsReporterJaeger(t *testing.T) {
	testClientMetrics(func(tr *clientMetricsTest) {
		defer tr.r.Close()

		batch := func(clientUUID *string, seqNo *int64) *jaeger.Batch {
			batch := &jaeger.Batch{
				Spans: []*jaeger.Span{{}},
				Process: &jaeger.Process{
					ServiceName: "blah",
				},
			}
			if clientUUID != nil {
				batch.Process.Tags = []*jaeger.Tag{{Key: "client-uuid", VStr: clientUUID}}
			}
			if seqNo != nil {
				batch.SeqNo = seqNo
			}
			return batch
		}
		blank := ""
		clientUUID := "foobar"
		seqNo := int64(1)

		tests := []struct {
			clientUUID *string
			seqNo      *int64
			expLog     string
		}{
			{},
			{clientUUID: &blank},
			{clientUUID: &clientUUID},
			{clientUUID: &clientUUID, seqNo: &seqNo, expLog: clientUUID},
		}

		for i, test := range tests {
			t.Run(fmt.Sprintf("iter%d", i), func(t *testing.T) {
				tr.logs.TakeAll()

				err := tr.r.EmitBatch(batch(test.clientUUID, test.seqNo))
				assert.NoError(t, err)
				assert.Len(t, tr.mr.Spans(), i+1)

				logs := tr.logs.FilterMessageSnippet("new client")
				if test.expLog == "" {
					assert.Equal(t, 0, logs.Len())
				} else {
					if assert.Equal(t, 1, logs.Len()) {
						field := logs.All()[0].ContextMap()["client-uuid"]
						assert.Equal(t, clientUUID, field, "client-uuid should be logged")
					}
				}
			})
		}
	})
}
