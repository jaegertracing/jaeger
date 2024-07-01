// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package datareceivers

import (
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

type kafkaDataReceiver struct {
	testbed.DataReceiverBase
	receiver receiver.Traces
}

func NewKafkaDataReceiver(port int) testbed.DataReceiver {
	return &kafkaDataReceiver{DataReceiverBase: testbed.DataReceiverBase{Port: port}}
}

func (dr *kafkaDataReceiver) Start(tc consumer.Traces, _ consumer.Metrics, _ consumer.Logs) error {
	factory := kafkareceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*kafkareceiver.Config)
	cfg.Brokers = []string{fmt.Sprintf("localhost:%d", dr.Port)}
	cfg.GroupID = "testbed_collector"

	var err error
	set := receivertest.NewNopSettings()
	dr.receiver, err = factory.CreateTracesReceiver(context.Background(), set, cfg, tc)
	if err != nil {
		return err
	}

	return dr.receiver.Start(context.Background(), componenttest.NewNopHost())
}

func (dr *kafkaDataReceiver) Stop() error {
	return dr.receiver.Shutdown(context.Background())
}

func (dr *kafkaDataReceiver) GenConfigYAMLStr() string {
	return fmt.Sprintf(`
  kafka:
    brokers:
      - localhost:%d
    encoding: otlp_proto`, dr.Port)
}

func (*kafkaDataReceiver) ProtocolName() string {
	return "kafka"
}
