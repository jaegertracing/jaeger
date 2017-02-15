// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
