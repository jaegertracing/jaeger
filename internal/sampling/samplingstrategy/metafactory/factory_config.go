// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metafactory

import (
	"fmt"
	"os"
)

const (
	// SamplingTypeEnvVar is the name of the env var that defines the type of sampling strategy store used.
	SamplingTypeEnvVar = "SAMPLING_CONFIG_TYPE"
)

// FactoryConfig tells the Factory what sampling type it needs to create.
type FactoryConfig struct {
	StrategyStoreType Kind
}

// FactoryConfigFromEnv reads the desired sampling type from the SAMPLING_CONFIG_TYPE environment variable. Allowed values:
// * `file` - built-in
// * `adaptive` - built-in
func FactoryConfigFromEnv() (*FactoryConfig, error) {
	strategyStoreType := getStrategyStoreTypeFromEnv()
	if strategyStoreType != samplingTypeAdaptive &&
		strategyStoreType != samplingTypeFile {
		return nil, fmt.Errorf("invalid sampling type: %s. Valid types are %v", strategyStoreType, AllSamplingTypes)
	}

	return &FactoryConfig{
		StrategyStoreType: Kind(strategyStoreType),
	}, nil
}

func getStrategyStoreTypeFromEnv() string {
	// check the new env var
	strategyStoreType := os.Getenv(SamplingTypeEnvVar)
	if strategyStoreType != "" {
		return strategyStoreType
	}

	// default
	return samplingTypeFile
}
