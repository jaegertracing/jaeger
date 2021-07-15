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

package nats

import (
	"testing"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestOptionsWithFlags(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--nats.producer.subject=topic1",
		"--nats.producer.brokers=127.0.0.1:9092, 0.0.0:1234",
		"--nats.producer.encoding=protobuf"})
	opts.InitFromViper(v)

	assert.Equal(t, "topic1", opts.subject)
	assert.Equal(t, []string{"127.0.0.1:9092", "0.0.0:1234"}, opts.config.Servers)
	assert.Equal(t, "protobuf", opts.encoding)
}

func TestFlagDefaults(t *testing.T) {
	opts := &Options{}
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v)

	assert.Equal(t, defaultSubject, opts.subject)
	assert.Equal(t, []string{defaultServer}, opts.config.Servers)
	assert.Equal(t, defaultEncoding, opts.encoding)
}


