// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
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

func (e *errHashable) Hash(io.Writer) error {
	return e.err
}

func TestHasCodeError(t *testing.T) {
	someErr := errors.New("some error")
	h := &errHashable{err: someErr}
	n, err := model.HashCode(h)
	assert.Equal(t, uint64(0), n)
	assert.Equal(t, someErr, err)
}
