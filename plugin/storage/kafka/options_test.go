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
		"--kafka.producer.encoding=protobuf"})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.config.Brokers)
	assert.Equal(t, "protobuf", opts.encoding)
}

func TestFlagDefaults(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Equal(t, defaultTopic, opts.topic)
	assert.Equal(t, []string{defaultBroker}, opts.config.Brokers)
	assert.Equal(t, defaultEncoding, opts.encoding)
}

func TestOptionsWithDeprecatedFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--kafka.topic=topic1",
		"--kafka.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.encoding=protobuf"})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.config.Brokers)
	assert.Equal(t, "protobuf", opts.encoding)
}

func TestOptionsWithAllFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--kafka.topic=topic1",
		"--kafka.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.encoding=protobuf",
		"--kafka.producer.topic=topic2",
		"--kafka.producer.brokers=10.0.0.1:9092, 10.0.0.2:9092",
		"--kafka.producer.encoding=json"})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.config.Brokers)
	assert.Equal(t, "protobuf", opts.encoding)
}
