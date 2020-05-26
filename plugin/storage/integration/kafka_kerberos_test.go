// Copyright (c) 2020 The Jaeger Authors.
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

// +build kafka_kerberos

package integration

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

const defaultLocalKafkaKerberosBroker = "localhost:9092"

type KafkaKeberosIntegrationTestSuite struct {
	StorageIntegration
	logger         *zap.Logger
	KerberosConfig string
}

func (s *KafkaKeberosIntegrationTestSuite) initialize() error {
	s.logger, _ = testutils.NewLogger()
	const encoding = "json"
	const groupID = "kafka-krb-integration-test"
	const clientID = "kafka-krb-integration-test"
	const kerberosRealm = "EXAMPLE.COM"
	const kerberosUserIngester = "ingester/localhost"
	const kerberosUserCollector = "collector/localhost"

	keyTabFileCollector := os.Getenv("KAFKA_KERBEROS_COLLECTOR_KEYTAB")
	keyTabFileIngester := os.Getenv("KAFKA_KERBEROS_INGESTER_KEYTAB")
	// A new topic is generated per execution to avoid data overlap
	topic := "jaeger-kafka-krb-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	f := kafka.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--kafka.producer.topic",
		topic,
		"--kafka.producer.brokers",
		defaultLocalKafkaKerberosBroker,
		"--kafka.producer.encoding",
		encoding,
		"--kafka.producer.kerberos.config-file",
		"/tmp/krb5.conf",
		"--kafka.producer.kerberos.keytab-file",
		keyTabFileCollector,
		"--kafka.producer.kerberos.realm",
		kerberosRealm,
		"--kafka.producer.kerberos.username",
		kerberosUserCollector,
		"--kafka.producer.kerberos.use-keytab",
		"true",
		"--kafka.producer.authentication",
		"kerberos",
	})
	if err != nil {
		return err
	}
	f.InitFromViper(v)
	if err := f.Initialize(metrics.NullFactory, s.logger); err != nil {
		return err
	}
	spanWriter, err := f.CreateSpanWriter()
	if err != nil {
		return err
	}

	v, command = config.Viperize(app.AddFlags)
	err = command.ParseFlags([]string{
		"--kafka.consumer.topic",
		topic,
		"--kafka.consumer.brokers",
		defaultLocalKafkaKerberosBroker,
		"--kafka.consumer.encoding",
		encoding,
		"--kafka.consumer.group-id",
		groupID,
		"--kafka.consumer.client-id",
		clientID,
		"--ingester.parallelism",
		"1000",
		"--kafka.consumer.kerberos.config-file",
		s.KerberosConfig,
		"--kafka.consumer.kerberos.keytab-file",
		keyTabFileIngester,
		"--kafka.consumer.kerberos.realm",
		kerberosRealm,
		"--kafka.consumer.kerberos.username",
		kerberosUserIngester,
		"--kafka.consumer.kerberos.use-keytab",
		"true",
		"--kafka.consumer.authentication",
		"kerberos",
	})
	if err != nil {
		return err
	}
	options := app.Options{}
	options.InitFromViper(v)
	traceStore := memory.NewStore()
	spanConsumer, err := builder.CreateConsumer(s.logger, metrics.NullFactory, traceStore, options)
	if err != nil {
		return err
	}
	spanConsumer.Start()

	s.SpanWriter = spanWriter
	s.SpanReader = &ingester{traceStore}
	s.Refresh = func() error { return nil }
	s.CleanUp = func() error { return nil }
	return nil
}

func TestKafkaKerberosStorage(t *testing.T) {
	s := &KafkaKeberosIntegrationTestSuite{
		KerberosConfig: os.Getenv("KAFKA_KERBEROS_CONFIG"),
	}
	require.NoError(t, s.initialize())
	t.Run("GetTrace", s.testGetTrace)
}
