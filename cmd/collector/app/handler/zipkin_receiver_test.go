// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestStartZipkinReceiver(t *testing.T) {
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
