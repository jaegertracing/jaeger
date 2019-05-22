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

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--kafka.producer.topic=topic1",
		"--kafka.producer.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.producer.encoding=protobuf",
		"--kafka.producer.required.acks=-1",
		"--kafka.producer.compression=1",
		"--kafka.producer.compression.level=6"})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.config.Brokers)
	assert.Equal(t, "protobuf", opts.encoding)
	assert.Equal(t, -1, opts.config.RequiredAcks)
	assert.Equal(t, 1, opts.config.Compression)
	assert.Equal(t, 6, opts.config.CompressionLevel)
}

func TestFlagDefaults(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Equal(t, defaultTopic, opts.topic)
	assert.Equal(t, []string{defaultBroker}, opts.config.Brokers)
	assert.Equal(t, defaultEncoding, opts.encoding)
	assert.Equal(t, defaultRequiredAcks, opts.config.RequiredAcks)
	assert.Equal(t, defaultComression, opts.config.Compression)
	assert.Equal(t, defaultCompressionLevel, opts.config.CompressionLevel)
}

func TestCompressionLevelDefaults(t *testing.T)  {
	assert.Equal(t, -1000, getCompressionLevel(0, -1000))
	assert.Equal(t, 6, getCompressionLevel(1, -1000))
	assert.Equal(t, -1000, getCompressionLevel(2, -10000 ))
	assert.Equal(t, -1000, getCompressionLevel(3, -1000))
	assert.Equal(t, 3, getCompressionLevel(4, -1000))

}
