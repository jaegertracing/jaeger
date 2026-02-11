// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jsonloader

import (
	"encoding/json"
	"fmt"
)

// LoadJSON calls loadFn to get raw bytes and unmarshals them
// into a value of type T.
func LoadJSON[T any](loadFn func() ([]byte, error)) (*T, error) {
	bytes, err := loadFn()
	if err != nil {
		return nil, err
	}

	var result *T
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}
	return result, nil
}
