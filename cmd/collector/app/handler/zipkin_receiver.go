// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"unsafe"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var (
	zipkinComponentType = component.MustNewType("zipkin")
	zipkinID            = component.NewID(zipkinComponentType)
)

// zipkinReceiverWrapper wraps the Opentelemetry zipkin receiver to apply keep alive settings
type zipkinReceiverWrapper struct {
	receiver.Traces
	keepAlive bool
	logger    *zap.Logger
}

// start wraps the original start method to apply keep-alive settings
func (w *zipkinReceiverWrapper) Start(ctx context.Context, host component.Host) error {
	err := w.Traces.Start(ctx, host)
	if err != nil {
		return err
	}

	if !w.keepAlive {
		if err := w.disableKeepAlive(); err != nil {
			w.logger.Warn("Failed to disable keep-alive on Zipkin receiver", zap.Error(err))
		} else {
			w.logger.Info("Disabled keep-alive on Zipkin receiver")
		}
	}

	return nil
}

// disableKeepAlive use reflection and unsafe operations to access the internal HTTP server and disable keep alive
func (w *zipkinReceiverWrapper) disableKeepAlive() error {
	receiverValue := reflect.ValueOf(w.Traces)
	if receiverValue.Kind() == reflect.Ptr {
		receiverValue = receiverValue.Elem()
	}

	serverField := receiverValue.FieldByName("server")
	if !serverField.IsValid() {
		return errors.New("server field not found in zipkin receiver")
	}

	if serverField.Kind() != reflect.Ptr || serverField.Type() != reflect.TypeOf((*http.Server)(nil)) {
		return errors.New("server field is not of type *http.Server")
	}

	if serverField.IsNil() {
		return errors.New("server field is nil")
	}

	serverPtr := (*http.Server)(unsafe.Pointer(serverField.Pointer()))
	if serverPtr == nil {
		return errors.New("server is nil")
	}
	serverPtr.SetKeepAlivesEnabled(false)
	w.logger.Debug("Successfully disabled keep-alive on Zipkin HTTP server")

	return nil
}

// StartZipkinReceiver starts Zipkin receiver from OTEL Collector.
func StartZipkinReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
) (receiver.Traces, error) {
	zipkinFactory := zipkinreceiver.NewFactory()
	return startZipkinReceiver(
		options,
		logger,
		spanProcessor,
		tm,
		zipkinFactory,
		consumer.NewTraces,
		zipkinFactory.CreateTraces,
	)
}

// Some of OTELCOL constructor functions return errors when passed nil arguments,
// which is a situation we cannot reproduce. To test our own error handling, this
// function allows to mock those constructors.
func startZipkinReceiver(
	options *flags.CollectorOptions,
	logger *zap.Logger,
	spanProcessor processor.SpanProcessor,
	tm *tenancy.Manager,
	// from here: params that can be mocked in tests
	zipkinFactory receiver.Factory,
	newTraces func(consume consumer.ConsumeTracesFunc, options ...consumer.Option) (consumer.Traces, error),
	createTracesReceiver func(ctx context.Context, set receiver.Settings,
		cfg component.Config, nextConsumer consumer.Traces) (receiver.Traces, error),
) (receiver.Traces, error) {
	receiverConfig := zipkinFactory.CreateDefaultConfig().(*zipkinreceiver.Config)
	receiverConfig.ServerConfig = options.Zipkin.ServerConfig
	receiverSettings := receiver.Settings{
		ID: zipkinID,
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: nooptrace.NewTracerProvider(),
			MeterProvider:  noopmetric.NewMeterProvider(),
		},
	}

	consumerHelper := &consumerHelper{
		batchConsumer: newBatchConsumer(logger,
			spanProcessor,
			processor.HTTPTransport,
			processor.ZipkinSpanFormat,
			tm),
	}

	nextConsumer, err := newTraces(consumerHelper.consume)
	if err != nil {
		return nil, fmt.Errorf("could not create Zipkin consumer: %w", err)
	}
	rcvr, err := createTracesReceiver(
		context.Background(),
		receiverSettings,
		receiverConfig,
		nextConsumer,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create Zipkin receiver: %w", err)
	}
	wrappedReceiver := &zipkinReceiverWrapper{ // wrap the receiver to apply keep-alive settings
		Traces:    rcvr,
		keepAlive: options.Zipkin.KeepAlive,
		logger:    logger,
	}

	if err := wrappedReceiver.Start(context.Background(), &otelHost{logger: logger}); err != nil {
		return nil, fmt.Errorf("could not start Zipkin receiver: %w", err)
	}
	return wrappedReceiver, nil
}
