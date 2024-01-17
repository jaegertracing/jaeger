// Copyright (c) 2024 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

type BadgerStorageConfig struct {
	BadgerDirectory       string        `mapstructure:"directory_key"`
	BadgerEphemeral       bool          `mapstructure:"ephemeral"`
	SpanStoreTTL          time.Duration `mapstructure:"span_store_ttl"`
	MaintenanceInterval   time.Duration `mapstructure:"maintenance_interval"`
	MetricsUpdateInterval time.Duration `mapstructure:"metrics_update_interval"`
}

type jaegerStorageDataReceiver struct {
	receiver receiver.Traces
	config   BadgerStorageConfig
}

func NewJaegerStorageDataReceiver(config BadgerStorageConfig) testbed.DataReceiver {
	return &jaegerStorageDataReceiver{
		config: config,
	}
}

func (dr *jaegerStorageDataReceiver) Start(tc consumer.Traces, _ consumer.Metrics, _ consumer.Logs) error {
	factory := storagereceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*storagereceiver.Config)

	cfg.Badger.KeyDirectory = dr.config.BadgerDirectory
	cfg.Badger.Ephemeral = dr.config.BadgerEphemeral

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
  jaeger_storage:
	badger:
	  directory_key: %s
	  maintenance_interval: %d
	  metrics_update_interval: %d`, dr.config.BadgerDirectory, time.Minute*dr.config.MaintenanceInterval, time.Second*dr.config.MetricsUpdateInterval)
}

func (dr *jaegerStorageDataReceiver) ProtocolName() string {
	return "jaeger_storage_receiver"
}
