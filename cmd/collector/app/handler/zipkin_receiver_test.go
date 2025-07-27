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
	"reflect"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	zipkinthrift "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin/zipkinthriftconverter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zipkin_proto3 "github.com/jaegertracing/jaeger/internal/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestZipkinReceiver(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	tm := &tenancy.Manager{}

	opts := &flags.CollectorOptions{}
	opts.Zipkin.Endpoint = ":11911"

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

	makeThrift := func(data []byte) []byte {
		var spans []*zipkincore.Span
		require.NoError(t, json.Unmarshal(data, &spans))
		out, err := zipkinthrift.SerializeThrift(context.Background(), spans)
		require.NoError(t, err)
		return out
	}

	makeProto := func(data []byte) []byte {
		var spans zipkin_proto3.ListOfSpans
		require.NoError(t, gogojsonpb.Unmarshal(bytes.NewReader(data), &spans))
		out, err := gogoproto.Marshal(&spans)
		require.NoError(t, err)
		return out
	}

	testCases := []struct {
		file     string
		prepFn   func(file []byte) []byte
		url      string
		encoding string
	}{
		{
			file:     "zipkin_thrift_v1_merged_spans.json",
			prepFn:   makeThrift,
			url:      "/api/v1/spans",
			encoding: "application/x-thrift",
		},
		{
			file:     "zipkin_proto_01.json",
			prepFn:   makeProto,
			url:      "/",
			encoding: "application/x-protobuf",
		},
		{
			file:     "zipkin_proto_02.json",
			url:      "/",
			prepFn:   makeProto,
			encoding: "application/x-protobuf",
		},
		{
			file: "zipkin_v1_merged_spans.json",
			url:  "/api/v1/spans",
		},
		{
			file: "zipkin_v2_01.json",
			url:  "/",
		},
		{
			file: "zipkin_v2_02.json",
			url:  "/",
		},
		{
			file: "zipkin_v2_03.json",
			url:  "/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile("./testdata/" + tc.file)
			require.NoError(t, err)
			if tc.prepFn != nil {
				data = tc.prepFn(data)
			}
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
			require.Equal(t, processor.ZipkinSpanFormat, spanProcessor.getSpanFormat())
		})
	}
}

func TestStartZipkinReceiver_Error(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	tm := &tenancy.Manager{}

	opts := &flags.CollectorOptions{}
	opts.Zipkin.Endpoint = ":-1"

	_, err := StartZipkinReceiver(opts, logger, spanProcessor, tm)
	require.ErrorContains(t, err, "could not start Zipkin receiver")

	newTraces := func(consumer.ConsumeTracesFunc, ...consumer.Option) (consumer.Traces, error) {
		return nil, errors.New("mock error")
	}
	f := zipkinreceiver.NewFactory()
	_, err = startZipkinReceiver(opts, logger, spanProcessor, tm, f, newTraces, f.CreateTraces)
	require.ErrorContains(t, err, "could not create Zipkin consumer")

	createTracesReceiver := func(
		context.Context, receiver.Settings, component.Config, consumer.Traces,
	) (receiver.Traces, error) {
		return nil, errors.New("mock error")
	}
	_, err = startZipkinReceiver(opts, logger, spanProcessor, tm, f, consumer.NewTraces, createTracesReceiver)
	assert.ErrorContains(t, err, "could not create Zipkin receiver")
}

func TestZipkinReceiverKeepAlive(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	tm := &tenancy.Manager{}

	testCases := []struct {
		name      string
		keepAlive bool
	}{
		{
			name:      "KeepAlive enabled",
			keepAlive: true,
		},
		{
			name:      "KeepAlive disabled",
			keepAlive: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := &flags.CollectorOptions{}
			opts.Zipkin.Endpoint = ":0"
			opts.Zipkin.KeepAlive = tc.keepAlive

			rec, err := StartZipkinReceiver(opts, logger, spanProcessor, tm)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, rec.Shutdown(context.Background()))
			}()

			wrapper, ok := rec.(*zipkinReceiverWrapper)
			require.True(t, ok, "receiver should be wrapped with zipkinReceiverWrapper")
			assert.Equal(t, tc.keepAlive, wrapper.keepAlive, "keepAlive setting should match")

			// Try to access the internal server to verify keep-alive setting
			// this is a test only verification using reflection
			receiverValue := reflect.ValueOf(wrapper.Traces)
			if receiverValue.Kind() == reflect.Ptr {
				receiverValue = receiverValue.Elem()
			}

			serverField := receiverValue.FieldByName("server")
			if serverField.IsValid() && !serverField.IsNil() {
				// we can not directly check the keep alive setting on the server
				// because it's an internal state but we can verify if our wrapper was applied correctly or not
				t.Logf("Server field found and is not nil for keepAlive=%v", tc.keepAlive)
			}
		})
	}
}

func TestZipkinReceiverWrapper_DisableKeepAlive_ErrorCases(t *testing.T) {
	logger, _ := testutils.NewLogger()

	tests := []struct {
		name           string
		receiver       receiver.Traces
		expectedErrMsg string
	}{
		{
			name:           "nil receiver",
			receiver:       nil,
			expectedErrMsg: "receiver is nil",
		},
		{
			name:           "receiver without server field",
			receiver:       &mockReceiverWithoutServerField{},
			expectedErrMsg: "server field not found in zipkin receiver",
		},
		{
			name:           "receiver with wrong server field type",
			receiver:       &mockReceiverWithWrongServerType{server: "not a server"},
			expectedErrMsg: "server field is not of type *http.Server",
		},
		{
			name:           "receiver with nil server field",
			receiver:       &mockReceiverWithNilServer{server: nil},
			expectedErrMsg: "server field is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := &zipkinReceiverWrapper{
				Traces:    tt.receiver,
				keepAlive: false,
				logger:    logger,
			}

			err := wrapper.disableKeepAlive()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErrMsg)
		})
	}
}

type mockReceiverWithoutServerField struct{}

func (*mockReceiverWithoutServerField) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (*mockReceiverWithoutServerField) Shutdown(_ context.Context) error {
	return nil
}

type mockReceiverWithWrongServerType struct {
	server string
}

func (*mockReceiverWithWrongServerType) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (*mockReceiverWithWrongServerType) Shutdown(_ context.Context) error {
	return nil
}

type mockReceiverWithNilServer struct {
	server *http.Server
}

func (*mockReceiverWithNilServer) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (*mockReceiverWithNilServer) Shutdown(_ context.Context) error {
	return nil
}
