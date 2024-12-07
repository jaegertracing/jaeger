// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

type Factory interface {
	CreateDependencyReader() (Reader, error)
}
