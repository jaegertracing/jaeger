package http

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	endpoint string
	mux      sync.Mutex
	requests []*http.Request
}

func (m *mockClient) Post(payload *bytes.Buffer) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	req, err := http.NewRequest(http.MethodPost, mockCollectorEndpoint, payload)
	m.requests = append(m.requests, req)
	return err
}

func TestReporter_EmitBatch(t *testing.T) {
	client := &mockClient{}
	builder := &Builder{Client: client, CollectorEndpoint: mockCollectorEndpoint}
	reporter, err := builder.CreateReporter()
	assert.NoError(t, err)

	ts := time.Unix(158, 0)
	batch := &jaeger.Batch{
		Process: &jaeger.Process{ServiceName: "reporter-test"},
		Spans: []*jaeger.Span{
			{OperationName: "foo", StartTime: int64(model.TimeAsEpochMicroseconds(ts))},
		},
	}
	serialized, err := serializeJaeger(batch)
	assert.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, mockCollectorEndpoint, serialized)
	tests := []struct {
		in       *jaeger.Batch
		expected *http.Request
		err      string
	}{
		{
			in:       batch,
			expected: req,
		},
	}

	for _, test := range tests {
		err := reporter.EmitBatch(test.in)
		if test.err != "" {
			assert.Equal(t, err, test.err)
		} else {
			require.NoError(t, err)
			r := client.requests[0]
			assert.Equal(t, test.expected.URL, r.URL)
			assert.Equal(t, test.expected.Body, r.Body)
		}
	}
}
