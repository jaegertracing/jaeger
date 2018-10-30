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

package throttling

import (
	"github.com/uber/tchannel-go/thrift"

	"github.com/jaegertracing/jaeger/cmd/collector/app/throttling/configstore"
	throttlingIDL "github.com/jaegertracing/jaeger/thrift-gen/throttling"
)

// Handler defines the interface for a throttling config service handler.
type Handler interface {
	// GetThrottlingConfigs returns the throttling configurations for the given
	// services. If the service name is not in the ServiceConfigs map, the
	// caller should use the DefaultConfig member instead.
	GetThrottlingConfigs(ctx thrift.Context, serviceNames []string) (*throttlingIDL.ThrottlingResponse, error)
}

type handler struct {
	store configstore.ConfigStore
}

// NewHandler creates a handler that serves throttling configs for services.
func NewHandler(store configstore.ConfigStore) Handler {
	return &handler{
		store: store,
	}
}

func (h *handler) GetThrottlingConfigs(ctx thrift.Context, serviceNames []string) (*throttlingIDL.ThrottlingResponse, error) {
	return h.store.GetThrottlingConfigs(serviceNames)
}
