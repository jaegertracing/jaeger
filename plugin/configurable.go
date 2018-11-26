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

package plugin

import (
	"flag"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/spf13/viper"
)

// Configurable interface can be implemented by plugins that require external configuration,
// such as CLI flags, config files, or environment variables.
type Configurable interface {
	// AddFlags adds CLI flags for configuring this component.
	AddFlags(flagSet *flag.FlagSet)

	// InitFromViper initializes this component with properties from spf13/viper.
	InitFromViper(v *viper.Viper)
}

type ConfigurableMetrics interface {
	AddMetrics(metricsFactory metrics.Factory)
}

type ConfigurableLogging interface {
	AddLogger(logger *zap.Logger)
}