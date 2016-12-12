// Copyright (c) 2016 Uber Technologies, Inc.
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

package multierror

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ExampleWrap() {
	someFunc := func() error {
		return errors.New("doh")
	}

	var errs []error
	for i := 0; i < 2; i++ {
		if err := someFunc(); err != nil {
			errs = append(errs, err)
		}
		fmt.Println(Wrap(errs).Error())
	}
	// Output: doh
	// [doh, doh]
}

func TestWrapEmptySlice(t *testing.T) {
	var errors []error
	e1 := Wrap(errors)
	assert.Nil(t, e1)
	e2 := Wrap([]error{})
	assert.Nil(t, e2)
}

func TestWrapSingleError(t *testing.T) {
	err := errors.New("doh")
	e1 := Wrap([]error{err})
	assert.Error(t, e1)
	assert.Equal(t, err, e1)
	assert.Equal(t, "doh", e1.Error())
}

func TestWrapManyErrors(t *testing.T) {
	err1 := errors.New("ay")
	err2 := errors.New("caramba")
	e1 := Wrap([]error{err1, err2})
	assert.Error(t, e1)
	assert.Equal(t, "[ay, caramba]", e1.Error())
}
