// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cache

// ServiceAliasMappingStorage interface for reading/writing service alias mapping from/to storage.
type ServiceAliasMappingStorage interface {
	Load() (map[string]string, error)
	Save(data map[string]string) error
}

// ServiceAliasMappingExternalSource interface to load service alias mapping from an external source
type ServiceAliasMappingExternalSource interface {
	Load() (map[string]string, error)
}
