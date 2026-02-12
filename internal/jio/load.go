// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package jio provides generic I/O utility functions.
package jio

import (
	"encoding/json"
	"fmt"
)

// JSONLoad calls the provided loader function to fetch raw bytes,
// then unmarshals the JSON data into a value of type T.
func JSONLoad[T any](loader func() ([]byte, error)) (*T, error) {
	data, err := loader()
	if err != nil {
		return nil, err
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return &v, nil
}
