// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// Service represents a single row in the ClickHouse `services` table.
type Service struct {
	Name string `ch:"name"`
}
