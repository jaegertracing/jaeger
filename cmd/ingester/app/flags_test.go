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

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

func TestOptionsWithFlags(t *testing.T) {
	o := &Options{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--kafka.consumer.topic=topic1",
		"--kafka.consumer.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--kafka.consumer.group-id=group1",
		"--kafka.consumer.encoding=json",
		"--ingester.parallelism=5",
		"--ingester.deadlockInterval=2m",
		"--ingester.http-port=2345"})
	o.InitFromViper(v)

	assert.Equal(t, "topic1", o.Topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, o.Brokers)
	assert.Equal(t, "group1", o.GroupID)
	assert.Equal(t, 5, o.Parallelism)
	assert.Equal(t, 2*time.Minute, o.DeadlockInterval)
	assert.Equal(t, kafka.EncodingJSON, o.Encoding)
	assert.Equal(t, 2345, o.IngesterHTTPPort)
}

func TestFlagDefaults(t *testing.T) {
	o := &Options{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{})
	o.InitFromViper(v)

	assert.Equal(t, DefaultTopic, o.Topic)
	assert.Equal(t, []string{DefaultBroker}, o.Brokers)
	assert.Equal(t, DefaultGroupID, o.GroupID)
	assert.Equal(t, DefaultParallelism, o.Parallelism)
	assert.Equal(t, DefaultEncoding, o.Encoding)
	assert.Equal(t, DefaultDeadlockInterval, o.DeadlockInterval)
}
