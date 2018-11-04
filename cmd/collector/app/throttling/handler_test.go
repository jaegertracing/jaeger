// Copyright (c) 2018 Uber Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/require"

	throttlingIDL "github.com/jaegertracing/jaeger/thrift-gen/throttling"
)

type mockStore struct {
	defaultConfig  throttlingIDL.ThrottlingConfig
	serviceConfigs map[string]throttlingIDL.ThrottlingConfig
}

func (m *mockStore) GetThrottlingConfigs(serviceNames []string) (*throttlingIDL.ThrottlingResponse, error) {
	resp := throttlingIDL.NewThrottlingResponse()
	resp.DefaultConfig = &m.defaultConfig
	for _, serviceName := range serviceNames {
		if serviceConfig, ok := m.serviceConfigs[serviceName]; ok {
			serviceThrottlingConfig := throttlingIDL.NewServiceThrottlingConfig()
			serviceThrottlingConfig.ServiceName = serviceName
			serviceThrottlingConfig.Config = &serviceConfig
			resp.ServiceConfigs = append(resp.ServiceConfigs, serviceThrottlingConfig)
		}
	}
	return resp, nil
}

func TestGetThrottlingConfigs(t *testing.T) {
	defaultConfig := throttlingIDL.ThrottlingConfig{
		MaxOperations:    2,
		CreditsPerSecond: 3.0,
		MaxBalance:       4.0,
	}
	serviceAConfig := throttlingIDL.ThrottlingConfig{
		MaxOperations:    1,
		CreditsPerSecond: 2.0,
		MaxBalance:       3.0,
	}

	store := &mockStore{
		defaultConfig: defaultConfig,
		serviceConfigs: map[string]throttlingIDL.ThrottlingConfig{
			"service-a": serviceAConfig,
		},
	}
	handler := NewHandler(store)

	response, err := handler.GetThrottlingConfigs(nil, []string{"service-a"})
	require.NoError(t, err)
	require.EqualValues(t, defaultConfig, *response.DefaultConfig)
	require.Len(t, response.ServiceConfigs, 1)
	serviceConfig := response.ServiceConfigs[0]
	require.Equal(t, "service-a", serviceConfig.ServiceName)
	require.EqualValues(t, serviceAConfig, *serviceConfig.Config)
}
