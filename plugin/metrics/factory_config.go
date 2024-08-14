// Copyright (c) 2021 The Jaeger Authors.
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

package metrics

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
