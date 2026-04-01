// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metricstest

import (
	"slices"
)

// GetKey converts name+tags into a single string of the form
// "name|tag1=value1|...|tagN=valueN", where tag names are
// sorted alphabetically.
func GetKey(name string, tags map[string]string, tagsSep string, tagKVSep string) string {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	key := name
	for _, k := range keys {
		key = key + tagsSep + k + tagKVSep + tags[k]
	}
	return key
}
