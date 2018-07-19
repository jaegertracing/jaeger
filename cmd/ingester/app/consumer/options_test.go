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

package consumer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--ingester-consumer.topic=topic1",
		"--ingester-consumer.brokers=127.0.0.1:9092,0.0.0:1234",
		"--ingester-consumer.group-id=group1",
		"--ingester-consumer.parallelism=5"})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.Topic)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.Brokers)
	assert.Equal(t, "group1", opts.GroupID)
	assert.Equal(t, 5, opts.Parallelism)
}

func TestFlagDefaults(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Equal(t, defaultTopic, opts.Topic)
	assert.Equal(t, []string{defaultBroker}, opts.Brokers)
	assert.Equal(t, defaultGroupID, opts.GroupID)
	assert.Equal(t, defaultParallelism, opts.Parallelism)
}
