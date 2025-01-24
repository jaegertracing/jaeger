// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var errIWillAlwaysFail = errors.New("ErrProneWriteSpanStore will always fail")

type errProneWriteSpanStore struct{}

func (*errProneWriteSpanStore) WriteSpan(context.Context, *model.Span) error {
	return errIWillAlwaysFail
}

type noopWriteSpanStore struct{}

func (*noopWriteSpanStore) WriteSpan(context.Context, *model.Span) error {
	return nil
}

func TestCompositeWriteSpanStoreSuccess(t *testing.T) {
	c := spanstore.NewCompositeWriter(&noopWriteSpanStore{}, &noopWriteSpanStore{})
	require.NoError(t, c.WriteSpan(context.Background(), nil))
}

func TestCompositeWriteSpanStoreSecondFailure(t *testing.T) {
	c := spanstore.NewCompositeWriter(&errProneWriteSpanStore{}, &errProneWriteSpanStore{})
	require.EqualError(t, c.WriteSpan(context.Background(), nil), fmt.Sprintf("%s\n%s", errIWillAlwaysFail, errIWillAlwaysFail))
}

func TestCompositeWriteSpanStoreFirstFailure(t *testing.T) {
	c := spanstore.NewCompositeWriter(&errProneWriteSpanStore{}, &noopWriteSpanStore{})
	require.EqualError(t, c.WriteSpan(context.Background(), nil), errIWillAlwaysFail.Error())
}
