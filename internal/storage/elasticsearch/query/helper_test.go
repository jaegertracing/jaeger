// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import "errors"

// errBadQuery is returned by errQuery to verify that compound nodes (bool,
// nested) propagate a child's Source error rather than swallowing it.
var errBadQuery = errors.New("bad query")

type errQuery struct{}

func (errQuery) Source() (any, error) { return nil, errBadQuery }

type errAgg struct{}

func (errAgg) Source() (any, error) { return nil, errBadQuery }
