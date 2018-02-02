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
	"os"
)

const (
	// SamplingTypeEnvVar is the name of the env var that defines the type of sampling strategy store used.
	SamplingTypeEnvVar = "SAMPLING_TYPE"
)

// FactoryConfig tells the Factory what sampling type it needs to create.
type FactoryConfig struct {
	StrategyStoreType string
}

// FactoryConfigFromEnv reads the desired sampling type from the SAMPLING_TYPE environment variable. Allowed values:
//   * `static` - built-in
//   * `adaptive` - built-in // TODO
func FactoryConfigFromEnv() FactoryConfig {
	strategyStoreType := os.Getenv(SamplingTypeEnvVar)
	if strategyStoreType == "" {
		strategyStoreType = staticStrategyStoreType
	}
	return FactoryConfig{
		StrategyStoreType: strategyStoreType,
	}
}
