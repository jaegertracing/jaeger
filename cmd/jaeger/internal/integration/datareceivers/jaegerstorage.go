// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package datareceivers

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/integration/receivers/storagereceiver"
)

type jaegerStorageDataReceiver struct {
	TraceStorage  string
	StorageConfig *jaegerstorage.Config
	host          *storagetest.StorageHost
	receiver      receiver.Traces
}

func NewJaegerStorageDataReceiver(traceStorage string, storageConfig *jaegerstorage.Config) testbed.DataReceiver {
	return &jaegerStorageDataReceiver{
		TraceStorage:  traceStorage,
		StorageConfig: storageConfig,
	}
}

func (dr *jaegerStorageDataReceiver) Start(tc consumer.Traces, _ consumer.Metrics, _ consumer.Logs) error {
	ctx := context.Background()

	extFactory := jaegerstorage.NewFactory()
	ext, err := extFactory.CreateExtension(ctx, extension.CreateSettings{
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		BuildInfo:         component.NewDefaultBuildInfo(),
	}, dr.StorageConfig)
	if err != nil {
		return err
	}

	rcvSet := receivertest.NewNopCreateSettings()
	rcvFactory := storagereceiver.NewFactory()
	rcvCfg := rcvFactory.CreateDefaultConfig().(*storagereceiver.Config)
	rcvCfg.TraceStorage = dr.TraceStorage
	rcv, err := rcvFactory.CreateTracesReceiver(ctx, rcvSet, rcvCfg, tc)
	if err != nil {
		return err
	}
	dr.receiver = rcv

	dr.host = storagetest.NewStorageHost()
	dr.host.WithExtension(jaegerstorage.ID, ext)

	err = dr.host.GetExtensions()[jaegerstorage.ID].Start(ctx, dr.host)
	if err != nil {
		return err
	}
	return dr.receiver.Start(ctx, dr.host)
}

func (dr *jaegerStorageDataReceiver) Stop() error {
	ctx := context.Background()
	err := dr.receiver.Shutdown(ctx)
	if err != nil {
		return err
	}
	return dr.host.GetExtensions()[jaegerstorage.ID].Shutdown(ctx)
}

func (dr *jaegerStorageDataReceiver) GenConfigYAMLStr() string {
	return fmt.Sprintf(`
  jaeger_storage_receiver:
    trace_storage: %s
`, dr.TraceStorage)
}

func (dr *jaegerStorageDataReceiver) ProtocolName() string {
	return "jaeger_storage_receiver"
}
