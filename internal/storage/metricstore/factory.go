// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

const (
	// disabledStorageType is the storage type used when METRICS_STORAGE_TYPE is unset.
	disabledStorageType = ""

	prometheusStorageType = "prometheus"
)

// AllStorageTypes defines all available storage backends.
var AllStorageTypes = []string{prometheusStorageType}
