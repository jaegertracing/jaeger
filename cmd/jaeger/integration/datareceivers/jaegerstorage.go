// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package datareceivers

import (
	"context"
	"fmt"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/integration/receivers/storagereceiver"
)

type jaegerStorageDataReceiver struct {
	Port     int
	receiver receiver.Traces
}

func NewJaegerStorageDataReceiver(port int) testbed.DataReceiver {
	return &jaegerStorageDataReceiver{Port: port}
}

func (dr *jaegerStorageDataReceiver) Start(tc consumer.Traces, _ consumer.Metrics, _ consumer.Logs) error {
	factory := storagereceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*storagereceiver.Config)
	cfg.GRPC.RemoteServerAddr = fmt.Sprintf("localhost:%d", dr.Port)
	cfg.GRPC.RemoteConnectTimeout = time.Duration(5 * time.Second)
	// TODO add support for other backends

	var err error
	set := receivertest.NewNopCreateSettings()
	dr.receiver, err = factory.CreateTracesReceiver(context.Background(), set, cfg, tc)
	if err != nil {
		return err
	}

	return dr.receiver.Start(context.Background(), componenttest.NewNopHost())
}

func (dr *jaegerStorageDataReceiver) Stop() error {
	return dr.receiver.Shutdown(context.Background())
}

func (dr *jaegerStorageDataReceiver) GenConfigYAMLStr() string {
	return fmt.Sprintf(`
  jaeger_storage_receiver:
    grpc:
	  server: localhost:%d`, dr.Port)
}

func (dr *jaegerStorageDataReceiver) ProtocolName() string {
	return "jaeger_storage_receiver"
}
