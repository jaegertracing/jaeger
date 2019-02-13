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

package strategystore

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

// Factory defines an interface for a factory that can create implementations of different strategy storage components.
// Implementations are also encouraged to implement plugin.Configurable interface.
//
// See also
//
// plugin.Configurable
type Factory interface {
	// Initialize performs internal initialization of the factory.
	Initialize(metricsFactory metrics.Factory, logger *zap.Logger, statr healthcheck.StatusReporter) error

	// CreateStrategyStore initializes the StrategyStore and returns it.
	CreateStrategyStore() (StrategyStore, error)
}
