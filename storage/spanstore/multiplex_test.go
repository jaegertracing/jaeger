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
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
	. "github.com/uber/jaeger/storage/spanstore"
)

var errIWillAlwaysFail = errors.New("ErrProneWriteSpanStore will always fail")

type errProneWriteSpanStore struct{}

func (e *errProneWriteSpanStore) WriteSpan(span *model.Span) error {
	return errIWillAlwaysFail
}

type noopWriteSpanStore struct{}

func (n *noopWriteSpanStore) WriteSpan(span *model.Span) error {
	return nil
}

func TestCompositeWriteSpanStoreSuccess(t *testing.T) {
	c := NewMultiplexWriter(&noopWriteSpanStore{}, &noopWriteSpanStore{})
	assert.NoError(t, c.WriteSpan(nil))
}

func TestCompositeWriteSpanStoreSecondFailure(t *testing.T) {
	c := NewMultiplexWriter(&errProneWriteSpanStore{}, &errProneWriteSpanStore{})
	assert.EqualError(t, c.WriteSpan(nil), fmt.Sprintf("[%s, %s]", errIWillAlwaysFail, errIWillAlwaysFail))
}

func TestCompositeWriteSpanStoreFirstFailure(t *testing.T) {
	c := NewMultiplexWriter(&errProneWriteSpanStore{}, &noopWriteSpanStore{})
	assert.Equal(t, errIWillAlwaysFail, c.WriteSpan(nil))
}
