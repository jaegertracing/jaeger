// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jtracer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestNew(t *testing.T) {
	jt, err := New("serviceName")
	require.NoError(t, err)
	require.NotNil(t, jt.OTEL, "Expected OTEL not to be nil")
	require.NotNil(t, jt.closer, "Expected closer not to be nil")

	jt.Close(context.Background())
}

func TestNoOp(t *testing.T) {
	jt := NoOp()
	require.NotNil(t, jt.OTEL)
	jt.Close(context.Background())
}

func TestNewHelperProviderError(t *testing.T) {
	fakeErr := errors.New("fakeProviderError")
	_, err := newHelper(
		"svc",
		func(_ context.Context, _ /* svc */ string) (*sdktrace.TracerProvider, error) {
			return nil, fakeErr
		})
	require.Error(t, err)
	require.EqualError(t, err, fakeErr.Error())
}

func TestInitHelperExporterError(t *testing.T) {
	fakeErr := errors.New("fakeExporterError")
	_, err := initHelper(
		context.Background(),
		"svc",
		func(_ context.Context) (sdktrace.SpanExporter, error) {
			return nil, fakeErr
		},
		func(_ context.Context, _ /* svc */ string) (*resource.Resource, error) {
			return nil, nil
		},
	)
	require.Error(t, err)
	require.EqualError(t, err, fakeErr.Error())
}

func TestInitHelperResourceError(t *testing.T) {
	fakeErr := errors.New("fakeResourceError")
	tp, err := initHelper(
		context.Background(),
		"svc",
		otelExporter,
		func(_ context.Context, _ /* svc */ string) (*resource.Resource, error) {
			return nil, fakeErr
		},
	)
	require.Error(t, err)
	require.Nil(t, tp)
	require.EqualError(t, err, fakeErr.Error())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
