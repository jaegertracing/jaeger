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
