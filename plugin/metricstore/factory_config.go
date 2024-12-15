// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"os"
)

const (
	// StorageTypeEnvVar is the name of the env var that defines the type of backend used for metrics storage.
	StorageTypeEnvVar = "METRICS_STORAGE_TYPE"
)

// FactoryConfig tells the Factory which types of backends it needs to create for different storage types.
type FactoryConfig struct {
	MetricsStorageType string
}

// FactoryConfigFromEnv reads the desired types of storage backends from METRICS_STORAGE_TYPE.
func FactoryConfigFromEnv() FactoryConfig {
	return FactoryConfig{
		MetricsStorageType: os.Getenv(StorageTypeEnvVar),
	}
}
