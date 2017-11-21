// Copyright (c) 2017 The Jaeger Authors.
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

package http

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

// Builder Struct to hold configurations
type Builder struct {
	scheme             string
	collectorHostPorts []string `yaml:"collectorHostPorts"`

	authToken string
	username  string
	password  string
}

// NewBuilder creates a new reporter builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithCollectorHostPorts sets the collectors hosts and ports to use
func (b *Builder) WithCollectorHostPorts(s []string) *Builder {
	b.collectorHostPorts = s
	return b
}

// WithScheme sets the protocol to use
func (b *Builder) WithScheme(s string) *Builder {
	b.scheme = s
	return b
}

// WithAuthToken sets the authentication token to use
func (b *Builder) WithAuthToken(s string) *Builder {
	b.authToken = s
	return b
}

// WithUsername sets the username to use
func (b *Builder) WithUsername(s string) *Builder {
	b.username = s
	return b
}

// WithPassword sets the password token to use
func (b *Builder) WithPassword(s string) *Builder {
	b.password = s
	return b
}

// CreateReporter creates the a reporter based on the configuration
func (b *Builder) CreateReporter(mFactory metrics.Factory, logger *zap.Logger) (*Reporter, error) {
	return New(b.scheme, b.collectorHostPorts, b.authToken, b.username, b.password, mFactory, logger)
}
