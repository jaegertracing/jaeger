// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package closers releases a list of io.Closer values as one.
package closers

import (
	"errors"
	"io"
)

// Closers is a list of io.Closer values that closes as one io.Closer.
type Closers []io.Closer

var _ io.Closer = Closers(nil)

// Close closes every entry in order and joins the errors, so one failure does
// not hide the rest.
func (cs Closers) Close() error {
	var errs []error
	for _, c := range cs {
		errs = append(errs, c.Close())
	}
	return errors.Join(errs...)
}
