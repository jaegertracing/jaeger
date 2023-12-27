// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	zipkinthrift "github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	zipkin_proto3 "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestZipkinReceiver(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	tm := &tenancy.Manager{}

	opts := &flags.CollectorOptions{}
	opts.Zipkin.HTTPHostPort = ":11911"

	rec, err := StartZipkinReceiver(opts, logger, spanProcessor, tm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rec.Shutdown(context.Background()))
	}()

	response, err := http.Post("http://localhost:11911/", "", nil)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	readJSON := func(file string) []byte {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		return data
	}

	readThrift := func(file string) []byte {
		var spans []*zipkincore.Span
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &spans))
		return zipkinthrift.SerializeThrift(context.Background(), spans)
	}

	readProto := func(file string) []byte {
		var spans zipkin_proto3.ListOfSpans
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		require.NoError(t, gogojsonpb.Unmarshal(bytes.NewReader(data), &spans))
		out, err := gogoproto.Marshal(&spans)
		require.NoError(t, err)
		return out
	}

	testCases := []struct {
		file     string
		prepFn   func(file string) []byte
		url      string
		encoding string
	}{
		{
			file:     "zipkin_thrift_v1_merged_spans.json",
			prepFn:   readThrift,
			url:      "/api/v1/spans",
			encoding: "application/x-thrift",
		},
		{
			file:     "zipkin_proto_01.json",
			prepFn:   readProto,
			url:      "/",
			encoding: "application/x-protobuf",
		},
		{
			file:     "zipkin_proto_02.json",
			url:      "/",
			prepFn:   readProto,
			encoding: "application/x-protobuf",
		},
		{
			file:   "zipkin_v1_merged_spans.json",
			prepFn: readJSON,
			url:    "/api/v1/spans",
		},
		{
			file:   "zipkin_v2_01.json",
			prepFn: readJSON,
			url:    "/",
		},
		{
			file:   "zipkin_v2_02.json",
			prepFn: readJSON,
			url:    "/",
		},
		{
			file:   "zipkin_v2_03.json",
			prepFn: readJSON,
			url:    "/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			data := tc.prepFn("./testdata/" + tc.file)
			response, err := http.Post(
				"http://localhost:11911"+tc.url,
				tc.encoding,
				bytes.NewReader(data),
			)
			require.NoError(t, err)
			assert.NotNil(t, response)
			if !assert.Equal(t, http.StatusAccepted, response.StatusCode) {
				bodyBytes, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				t.Logf("response: %s %s", response.Status, string(bodyBytes))
			}
			require.NoError(t, response.Body.Close())
		})
	}

	// TODO add more tests by submitting different formats of spans (instead of relying)
}

func TestStartZipkinReceiver_Error(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	tm := &tenancy.Manager{}

	opts := &flags.CollectorOptions{}
	opts.Zipkin.HTTPHostPort = ":-1"

	_, err := StartZipkinReceiver(opts, logger, spanProcessor, tm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not start Zipkin receiver")

	newTraces := func(consumer.ConsumeTracesFunc, ...consumer.Option) (consumer.Traces, error) {
		return nil, errors.New("mock error")
	}
	f := zipkinreceiver.NewFactory()
	_, err = startZipkinReceiver(opts, logger, spanProcessor, tm, f, newTraces, f.CreateTracesReceiver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create Zipkin consumer")

	createTracesReceiver := func(
		context.Context, receiver.CreateSettings, component.Config, consumer.Traces,
	) (receiver.Traces, error) {
		return nil, errors.New("mock error")
	}
	_, err = startZipkinReceiver(opts, logger, spanProcessor, tm, f, consumer.NewTraces, createTracesReceiver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create Zipkin receiver")
}
