// Copyright (c) 2019 The Jaeger Authors.
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
	"os"

	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
)

// TracerConfig initializes jaeger client config from env with defaults for jaeger-query
func TracerConfig() (*jaegerClientConfig.Configuration, error) {
	cfg, err := jaegerClientConfig.FromEnv()
	if err != nil {
		return nil, err
	}
	// backwards compatibility
	if e := os.Getenv("JAEGER_RPC_METRICS"); e == "" {
		cfg.RPCMetrics = true
	}
	if e := os.Getenv("JAEGER_SAMPLER_TYPE"); e == "" {
		cfg.Sampler.Type = "probabilistic"
	}
	if e := os.Getenv("JAEGER_SAMPLER_PARAM"); e == "" {
		cfg.Sampler.Param = 1.0
	}
	return cfg, nil
}
