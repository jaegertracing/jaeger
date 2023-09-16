// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelcmd

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/spanmetricsprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/spanprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/connector/forwardconnector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/loggingexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/zpagesextension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func components() (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}

	factories.Extensions, err = extension.MakeFactoryMap(
		// standard
		// ballastextension.NewFactory(),
		zpagesextension.NewFactory(),
		// add-ons
		// TODO add adaptive sampling
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Receivers, err = receiver.MakeFactoryMap(
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

	factories.Exporters, err = exporter.MakeFactoryMap(
		// standard
		loggingexporter.NewFactory(),
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
		// add-ons
		// elasticsearch.NewFactory(),
		kafkaexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Processors, err = processor.MakeFactoryMap(
		// standard
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
		// add-ons
		attributesprocessor.NewFactory(),
		resourcedetectionprocessor.NewFactory(),
		resourceprocessor.NewFactory(),
		spanmetricsprocessor.NewFactory(),
		spanprocessor.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Connectors, err = connector.MakeFactoryMap(
		// standard
		forwardconnector.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	return factories, nil
}
