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
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"

	"github.com/uber/jaeger/pkg/es"
)

// Configuration describes the configuration properties needed to connect to an ElasticSearch cluster
type Configuration struct {
	Servers     []string
	Username    string
	Password    string
	Sniffer     bool          // https://github.com/olivere/elastic/wiki/Sniffing
	MaxSpanAge  time.Duration `yaml:"max_span_age"` // configures the maximum lookback on span reads
	NumShards   int64         `yaml:"shards"`
	NumReplicas int64         `yaml:"replicas"`
}

// ClientBuilder creates new es.Client
type ClientBuilder interface {
	NewClient() (es.Client, error)
	GetNumShards() int64
	GetNumReplicas() int64
	GetMaxSpanAge() time.Duration
}

// NewClient creates a new ElasticSearch client
func (c *Configuration) NewClient() (es.Client, error) {
	if len(c.Servers) < 1 {
		return nil, errors.New("No servers specified")
	}
	rawClient, err := elastic.NewClient(c.GetConfigs()...)
	if err != nil {
		return nil, err
	}
	return es.WrapESClient(rawClient), nil
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
	if c.Username == "" {
		c.Username = source.Username
	}
	if c.Password == "" {
		c.Password = source.Password
	}
	if c.Sniffer == false {
		c.Sniffer = source.Sniffer
	}
	if c.MaxSpanAge == 0 {
		c.MaxSpanAge = source.MaxSpanAge
	}
	if c.NumShards == 0 {
		c.NumShards = source.NumShards
	}
	if c.NumReplicas == 0 {
		c.NumReplicas = source.NumReplicas
	}
}

// GetNumShards returns number of shards from Configuration
func (c *Configuration) GetNumShards() int64 {
	return c.NumShards
}

// GetNumReplicas returns number of replicas from Configuration
func (c *Configuration) GetNumReplicas() int64 {
	return c.NumReplicas
}

// GetMaxSpanAge returns max span age from Configuration
func (c *Configuration) GetMaxSpanAge() time.Duration {
	return c.MaxSpanAge
}

// GetConfigs wraps the configs to feed to the ElasticSearch client init
func (c *Configuration) GetConfigs() []elastic.ClientOptionFunc {
	options := make([]elastic.ClientOptionFunc, 3)
	options[0] = elastic.SetURL(c.Servers...)
	options[1] = elastic.SetBasicAuth(c.Username, c.Password)
	options[2] = elastic.SetSniff(c.Sniffer)
	return options
}
