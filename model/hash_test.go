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

package model_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/model"
)

type mockHashWwiterAnswer struct {
	n   int
	err error
}

type mockHashWwiter struct {
	answers []mockHashWwiterAnswer
}

func (w *mockHashWwiter) Write(data []byte) (int, error) {
	if len(w.answers) == 0 {
		return 0, fmt.Errorf("no answer registered for call with data=%+v", data)
	}
	answer := w.answers[0]
	w.answers = w.answers[1:]
	return answer.n, answer.err
}

type errHashable struct {
	err error
}

func (e *errHashable) Hash(w io.Writer) error {
	return e.err
}

func TestHasCodeError(t *testing.T) {
	someErr := errors.New("some error")
	h := &errHashable{err: someErr}
	n, err := model.HashCode(h)
	assert.Equal(t, uint64(0), n)
	assert.Equal(t, someErr, err)
}
