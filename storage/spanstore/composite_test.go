// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	. "github.com/jaegertracing/jaeger/storage/spanstore"
)

var errIWillAlwaysFail = errors.New("ErrProneWriteSpanStore will always fail")

type errProneWriteSpanStore struct{}

func (e *errProneWriteSpanStore) WriteSpan(ctx context.Context, span *model.Span) error {
	return errIWillAlwaysFail
}

type noopWriteSpanStore struct{}

func (n *noopWriteSpanStore) WriteSpan(ctx context.Context, span *model.Span) error {
	return nil
}

func TestCompositeWriteSpanStoreSuccess(t *testing.T) {
	c := NewCompositeWriter(&noopWriteSpanStore{}, &noopWriteSpanStore{})
	require.NoError(t, c.WriteSpan(context.Background(), nil))
}

func TestCompositeWriteSpanStoreSecondFailure(t *testing.T) {
	c := NewCompositeWriter(&errProneWriteSpanStore{}, &errProneWriteSpanStore{})
	require.EqualError(t, c.WriteSpan(context.Background(), nil), fmt.Sprintf("%s\n%s", errIWillAlwaysFail, errIWillAlwaysFail))
}

func TestCompositeWriteSpanStoreFirstFailure(t *testing.T) {
	c := NewCompositeWriter(&errProneWriteSpanStore{}, &noopWriteSpanStore{})
	require.EqualError(t, c.WriteSpan(context.Background(), nil), errIWillAlwaysFail.Error())
}
