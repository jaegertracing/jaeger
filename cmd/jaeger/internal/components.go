// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"

	"github.com/jaegertracing/jaeger/cmd/jaeger/components/connector/forwardconnector"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/connector/spanmetricsconnector"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/debugexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/kafkaexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/nopexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/otlpexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/otlphttpexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/prometheusexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/basicauthextension"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/expvar"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/healthcheckv2extension"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegermcp"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/pprofextension"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/remotesampling"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/remotestorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/sigv4authextension"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/zpagesextension"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/adaptivesampling"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/attributesprocessor"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/batchprocessor"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/filterprocessor"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/memorylimiterprocessor"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/tailsamplingprocessor"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/receiver/jaegerreceiver"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/receiver/kafkareceiver"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/receiver/nopreceiver"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/receiver/otlpreceiver"
	"github.com/jaegertracing/jaeger/cmd/jaeger/components/receiver/zipkinreceiver"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/storagecleaner"
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
	factories := otelcol.Factories{
		Telemetry: WrapFactory(otelconftelemetry.NewFactory()),
	}

	factories.Extensions, err = b.extension(
		// standard
		healthcheckv2extension.NewFactory(),
		pprofextension.NewFactory(),
		zpagesextension.NewFactory(),

		// add-ons
		basicauthextension.NewFactory(),
		sigv4authextension.NewFactory(),
		jaegermcp.NewFactory(),
		jaegerquery.NewFactory(),
		jaegerstorage.NewFactory(),
		remotesampling.NewFactory(),
		expvar.NewFactory(),
		// only for e2e testing
		storagecleaner.NewFactory(),
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
		filterprocessor.NewFactory(),
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
