// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/connector/forwardconnector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/loggingexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/ballastextension"
	"go.opentelemetry.io/collector/extension/zpagesextension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
)

func getOtelcolFactories(
	extension func(factories ...extension.Factory) (map[component.Type]extension.Factory, error),
	receiver func(factories ...receiver.Factory) (map[component.Type]receiver.Factory, error),
	exporter func(factories ...exporter.Factory) (map[component.Type]exporter.Factory, error),
	processor func(factories ...processor.Factory) (map[component.Type]processor.Factory, error),
	connector func(factories ...connector.Factory) (map[component.Type]connector.Factory, error),
) (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}

	factories.Extensions, err = extension(
		// standard
		ballastextension.NewFactory(),
		zpagesextension.NewFactory(),
		// add-ons
		jaegerquery.NewFactory(),
		jaegerstorage.NewFactory(),
		// TODO add adaptive sampling
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Receivers, err = receiver(
		// standard
		otlpreceiver.NewFactory(),
		// add-ons
		jaegerreceiver.NewFactory(),
		kafkareceiver.NewFactory(),
		zipkinreceiver.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Exporters, err = exporter(
		// standard
		loggingexporter.NewFactory(),
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
		// add-ons
		storageexporter.NewFactory(), // generic exporter to Jaeger v1 spanstore.SpanWriter
		kafkaexporter.NewFactory(),
		// elasticsearch.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Processors, err = processor(
		// standard
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
		// add-ons
		// TODO add adaptive sampling
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Connectors, err = connector(
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

func components() (otelcol.Factories, error) {
	return getOtelcolFactories(
		extension.MakeFactoryMap,
		receiver.MakeFactoryMap,
		exporter.MakeFactoryMap,
		processor.MakeFactoryMap,
		connector.MakeFactoryMap)
}
