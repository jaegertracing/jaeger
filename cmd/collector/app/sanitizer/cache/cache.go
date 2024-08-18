// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cache

// Cache interface for a string to string cache
type Cache interface {
	Get(key string) string
	Put(key string, value string) error
	IsEmpty() bool
	Initialize() error
}
