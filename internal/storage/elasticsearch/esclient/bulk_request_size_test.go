// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metricstest"
)

// stubTransport is an esapi.Transport that records the request it receives and
// answers with a fixed response or error.
type stubTransport struct {
	got *http.Request
	res *http.Response
	err error
}

func (s *stubTransport) Perform(req *http.Request) (*http.Response, error) {
	s.got = req
	return s.res, s.err
}

func newRequestSizeTransport(t *testing.T, next *stubTransport) (*requestSizeTransport, *metricstest.Factory) {
	mf := metricstest.NewFactory(time.Second)
	t.Cleanup(mf.Stop)
	return &requestSizeTransport{
		next:         next,
		requestBytes: newRequestBytesHistogram(mf),
	}, mf
}

func TestRequestSizeTransportRecordsContentLength(t *testing.T) {
	next := &stubTransport{res: &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}}
	tr, mf := newRequestSizeTransport(t, next)
	body := bytes.NewBufferString(`{"index":{}}` + "\n" + `{"a":1}` + "\n")
	req, err := http.NewRequest(http.MethodPost, "/_bulk", body)
	require.NoError(t, err)
	require.Equal(t, int64(21), req.ContentLength, "http.NewRequest sets ContentLength for a bytes.Buffer")

	res, err := tr.Perform(req)
	require.NoError(t, err)
	assert.Same(t, next.res, res)
	assert.Same(t, req, next.got, "the request is delegated unchanged")
	assertRequestBytes(t, mf, 21)
}

func TestRequestSizeTransportSkipsUnknownContentLength(t *testing.T) {
	next := &stubTransport{res: &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}}
	tr, mf := newRequestSizeTransport(t, next)
	// A streaming body of unknown length leaves ContentLength at 0 with Body set,
	// which net/http defines as unknown.
	req, err := http.NewRequest(http.MethodPost, "/_bulk", io.NopCloser(bytes.NewBufferString("x")))
	require.NoError(t, err)
	require.Equal(t, int64(0), req.ContentLength)
	require.NotNil(t, req.Body)

	_, err = tr.Perform(req)
	require.NoError(t, err)
	_, gauges := mf.Snapshot()
	assert.False(t, hasTimer(gauges, "bulk_index.request-bytes"), "an unknown length is not recorded: %v", gauges)
}

func TestRequestSizeTransportDelegatesError(t *testing.T) {
	next := &stubTransport{err: errors.New("dial failed")}
	tr, mf := newRequestSizeTransport(t, next)
	req, err := http.NewRequest(http.MethodPost, "/_bulk", bytes.NewBufferString("abc"))
	require.NoError(t, err)

	res, err := tr.Perform(req)
	require.EqualError(t, err, "dial failed")
	assert.Nil(t, res)
	// The size is recorded before the request is sent, so a failed request still counts.
	assertRequestBytes(t, mf, 3)
}

// assertRequestBytes asserts that bulk_index.request-bytes holds exactly one
// sample of the given size. metricstest renders a histogram as percentile gauges,
// so a single sample shows as every percentile equal to it; a second, different
// sample would pull P50 and P999 apart.
func assertRequestBytes(t *testing.T, mf *metricstest.Factory, size int) {
	t.Helper()
	require.Positive(t, size)
	_, gauges := mf.Snapshot()
	for _, p := range []string{"P50", "P75", "P90", "P95", "P99", "P999"} {
		assert.Equal(t, int64(size), gauges["bulk_index.request-bytes."+p], "gauges: %v", gauges)
	}
}
