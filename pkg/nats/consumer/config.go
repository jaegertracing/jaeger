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
	nats "github.com/nats-io/nats.go"
)

// Consumer is an interface to features of NATS that are necessary for the consumer
type Consumer interface {
}

// Builder builds a new nats consumer
type Builder interface {
	NewConsumer() (Consumer, error)
}

// Configuration describes the configuration properties needed to create a NATS consumer
type Configuration struct {
	Consumer

	Servers         string
	Subject         string
}

// NewConsumer creates a new NATS consumer
func (c *Configuration) NewConsumer() (Consumer, error) {
	nc, err := nats.Connect(c.Servers)
	return nc, err
}
