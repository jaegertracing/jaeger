// Copyright (c) 2018 The Jaeger Authors.
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

package kafka

import (
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--kafka.producer.topic=topic1",
		"--kafka.producer.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.producer.encoding=protobuf",
		"--kafka.producer.required-acks=local",
		"--kafka.producer.compression=gzip",
		"--kafka.producer.compression-level=7",
		"--kafka.producer.batch-linger=1s",
		"--kafka.producer.batch-size=128000",
		"--kafka.producer.batch-max-messages=100",
	})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.config.Brokers)
	assert.Equal(t, "protobuf", opts.encoding)
	assert.Equal(t, sarama.WaitForLocal, opts.config.RequiredAcks)
	assert.Equal(t, sarama.CompressionGZIP, opts.config.Compression)
	assert.Equal(t, 7, opts.config.CompressionLevel)
	assert.Equal(t, 128000, opts.config.BatchSize)
	assert.Equal(t, time.Duration(1*time.Second), opts.config.BatchLinger)
	assert.Equal(t, 100, opts.config.BatchMaxMessages)
}

func TestFlagDefaults(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Equal(t, defaultTopic, opts.topic)
	assert.Equal(t, []string{defaultBroker}, opts.config.Brokers)
	assert.Equal(t, defaultEncoding, opts.encoding)
	assert.Equal(t, sarama.WaitForLocal, opts.config.RequiredAcks)
	assert.Equal(t, sarama.CompressionNone, opts.config.Compression)
	assert.Equal(t, 0, opts.config.CompressionLevel)
	assert.Equal(t, 0, opts.config.BatchSize)
	assert.Equal(t, time.Duration(0*time.Second), opts.config.BatchLinger)
	assert.Equal(t, 0, opts.config.BatchMaxMessages)
}

func TestCompressionLevelDefaults(t *testing.T) {
	compressionLevel, err := getCompressionLevel("none", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["none"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("gzip", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["gzip"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("snappy", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["snappy"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("lz4", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["lz4"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("zstd", defaultCompressionLevel)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["zstd"].defaultCompressionLevel, compressionLevel)
}

func TestCompressionLevel(t *testing.T) {
	compressionLevel, err := getCompressionLevel("none", 0)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["none"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("gzip", 4)
	require.NoError(t, err)
	assert.Equal(t, 4, compressionLevel)

	compressionLevel, err = getCompressionLevel("snappy", 0)
	require.NoError(t, err)
	assert.Equal(t, compressionModes["snappy"].defaultCompressionLevel, compressionLevel)

	compressionLevel, err = getCompressionLevel("lz4", 10)
	require.NoError(t, err)
	assert.Equal(t, 10, compressionLevel)

	compressionLevel, err = getCompressionLevel("zstd", 20)
	require.NoError(t, err)
	assert.Equal(t, 20, compressionLevel)
}

func TestFailedCompressionLevelScenario(t *testing.T) {
	_, err := getCompressionLevel("gzip", 14)
	assert.Error(t, err)

	_, err = getCompressionLevel("lz4", 18)
	assert.Error(t, err)

	_, err = getCompressionLevel("zstd", 25)
	assert.Error(t, err)

	_, err = getCompressionLevel("test", 1)
	assert.Error(t, err)
}

func TestCompressionModes(t *testing.T) {
	compressionModes, err := getCompressionMode("gzip")
	require.NoError(t, err)
	assert.Equal(t, sarama.CompressionGZIP, compressionModes)

	compressionModes, err = getCompressionMode("snappy")
	require.NoError(t, err)
	assert.Equal(t, sarama.CompressionSnappy, compressionModes)

	compressionModes, err = getCompressionMode("none")
	require.NoError(t, err)
	assert.Equal(t, sarama.CompressionNone, compressionModes)
}

func TestCompressionModeFailures(t *testing.T) {
	_, err := getCompressionMode("test")
	assert.Error(t, err)
}

func TestRequiredAcks(t *testing.T) {
	acks, err := getRequiredAcks("noack")
	require.NoError(t, err)
	assert.Equal(t, sarama.NoResponse, acks)

	acks, err = getRequiredAcks("local")
	require.NoError(t, err)
	assert.Equal(t, sarama.WaitForLocal, acks)

	acks, err = getRequiredAcks("all")
	require.NoError(t, err)
	assert.Equal(t, sarama.WaitForAll, acks)
}

func TestRequiredAcksFailures(t *testing.T) {
	_, err := getRequiredAcks("test")
	assert.Error(t, err)
}
