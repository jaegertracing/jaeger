// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckv2extension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/connector/forwardconnector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/nopexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/zpagesextension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/nopreceiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/expvar"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotestorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/processors/adaptivesampling"
)

type builders struct {
	extension func(factories ...extension.Factory) (map[component.Type]extension.Factory, error)
	receiver  func(factories ...receiver.Factory) (map[component.Type]receiver.Factory, error)
	exporter  func(factories ...exporter.Factory) (map[component.Type]exporter.Factory, error)
	processor func(factories ...processor.Factory) (map[component.Type]processor.Factory, error)
	connector func(factories ...connector.Factory) (map[component.Type]connector.Factory, error)
}

func defaultBuilders() builders {
	return builders{
		extension: otelcol.MakeFactoryMap[extension.Factory],
		receiver:  otelcol.MakeFactoryMap[receiver.Factory],
		exporter:  otelcol.MakeFactoryMap[exporter.Factory],
		processor: otelcol.MakeFactoryMap[processor.Factory],
		connector: otelcol.MakeFactoryMap[connector.Factory],
	}
}

func (b builders) build() (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}

	factories.Extensions, err = b.extension(
		// standard
		zpagesextension.NewFactory(),
		healthcheckv2extension.NewFactory(),
		// add-ons
		jaegerquery.NewFactory(),
		jaegerstorage.NewFactory(),
		storagecleaner.NewFactory(),
		remotesampling.NewFactory(),
		expvar.NewFactory(),
		remotestorage.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Receivers, err = b.receiver(
		// standard
		otlpreceiver.NewFactory(),
		nopreceiver.NewFactory(),
		// add-ons
		jaegerreceiver.NewFactory(),
		kafkareceiver.NewFactory(),
		zipkinreceiver.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Exporters, err = b.exporter(
		// standard
		debugexporter.NewFactory(),
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
		nopexporter.NewFactory(),
		// add-ons
		storageexporter.NewFactory(), // generic exporter to Jaeger v1 spanstore.SpanWriter
		kafkaexporter.NewFactory(),
		prometheusexporter.NewFactory(),
		// elasticsearch.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Processors, err = b.processor(
		// standard
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
		tailsamplingprocessor.NewFactory(),
		attributesprocessor.NewFactory(),
		// add-ons
		adaptivesampling.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Connectors, err = b.connector(
		// standard
		forwardconnector.NewFactory(),
		// add-ons
		spanmetricsconnector.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	return factories, nil
}

func Components() (otelcol.Factories, error) {
	return defaultBuilders().build()
}
