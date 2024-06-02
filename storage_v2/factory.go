// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storage_v2

import (
	"context"
)

// Factory is a general factory interface to be reused across different storage factories.
// It lives within the OTEL collector extension component's lifecycle.
// The Initialize and Close functions supposed to be called from the
// OTEL component's Start and Shutdown functions.
type FactoryBase interface {
	// Initialize performs internal initialization of the factory,
	// such as opening connections to the backend store.
	Initialize(ctx context.Context) error

	// Close closes the resources held by the factory
	Close(ctx context.Context) error
}
