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
	"fmt"
	"io"
	"os"
)

const (
	// SamplingTypeEnvVar is the name of the env var that defines the type of sampling strategy store used.
	SamplingTypeEnvVar = "SAMPLING_CONFIG_TYPE"

	// previously the SAMPLING_TYPE env var was used for configuration, continue to support this old env var with warnings
	deprecatedSamplingTypeEnvVar = "SAMPLING_TYPE"
	// static is the old name for "file". we will translate from the deprecated to current name here. all other code will expect "file"
	deprecatedSamplingTypeStatic = "static"
)

// FactoryConfig tells the Factory what sampling type it needs to create.
type FactoryConfig struct {
	StrategyStoreType Kind
}

// FactoryConfigFromEnv reads the desired sampling type from the SAMPLING_CONFIG_TYPE environment variable. Allowed values:
// * `file` - built-in
// * `adaptive` - built-in
func FactoryConfigFromEnv(log io.Writer) (*FactoryConfig, error) {
	strategyStoreType := getStrategyStoreTypeFromEnv(log)
	if strategyStoreType != samplingTypeAdaptive &&
		strategyStoreType != samplingTypeFile {
		return nil, fmt.Errorf("invalid sampling type: %s. Valid types are %v", strategyStoreType, AllSamplingTypes)
	}

	return &FactoryConfig{
		StrategyStoreType: Kind(strategyStoreType),
	}, nil
}

func getStrategyStoreTypeFromEnv(log io.Writer) string {
	// check the new env var
	strategyStoreType := os.Getenv(SamplingTypeEnvVar)
	if strategyStoreType != "" {
		return strategyStoreType
	}

	// accept the old env var and value but warn
	strategyStoreType = os.Getenv(deprecatedSamplingTypeEnvVar)
	if strategyStoreType != "" {
		fmt.Fprintf(log, "WARNING: Using deprecated '%s' env var. Please switch to '%s'.\n", deprecatedSamplingTypeEnvVar, SamplingTypeEnvVar)
		if strategyStoreType == deprecatedSamplingTypeStatic {
			fmt.Fprintf(log, "WARNING: Using deprecated '%s' value for %s. Please switch to '%s'.\n", strategyStoreType, SamplingTypeEnvVar, samplingTypeFile)
			strategyStoreType = samplingTypeFile
		}
		return strategyStoreType
	}

	// default
	return samplingTypeFile
}
