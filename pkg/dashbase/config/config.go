// Copyright (c) 2017 Uber Technologies, Inc.
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

package config

import (
	"github.com/pkg/errors"
	"github.com/jaegertracing/jaeger/pkg/dashbase"
)

// Configuration describes the configuration properties needed to connect to an ElasticSearch cluster

type Configuration struct {
	Server     string
	KafkaHost  []string
	KafkaTopic string
}

// ClientBuilder creates new es.Client
type Builder interface {
	NewKafkaClient() (dashbase.KafkaClient, error)
	GetKafkaTopic() string
	GetService() string
}

// NewClient creates a new ElasticSearch client
func (c *Configuration) NewKafkaClient() (dashbase.KafkaClient, error) {
	kafkaClient := dashbase.KafkaClient{Hosts:c.KafkaHost}
	if len(c.KafkaHost) < 1 {
		return kafkaClient, errors.New("No servers specified")
	}
	err := kafkaClient.Open()
	if err != nil {
		return kafkaClient, err
	}
	return kafkaClient, nil
}

// GetNumShards returns number of shards from Configuration
func (c *Configuration) GetKafkaTopic() string {
	return c.KafkaTopic
}

func (c *Configuration) GetService() string {
	return c.Server
}