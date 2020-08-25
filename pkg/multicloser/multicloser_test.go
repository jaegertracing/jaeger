// Copyright (c) 2020 The Jaeger Authors.
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

package multicloser

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloser(t *testing.T) {
	expectedErr := "some error"
	tests := []struct {
		closer      io.Closer
		expectedErr string
	}{
		{
			closer: Wrap(testCloser{}, testCloser{}),
		},
		{
			closer:      Wrap(testCloser{}, testCloser{fmt.Errorf(expectedErr)}),
			expectedErr: expectedErr,
		},
		{
			closer:      Wrap(testCloser{fmt.Errorf(expectedErr)}, testCloser{fmt.Errorf(expectedErr)}),
			expectedErr: fmt.Sprintf("[%v, %v]", expectedErr, expectedErr),
		},
		{
			closer: Wrap(nil),
		},
	}
	for _, test := range tests {
		err := test.closer.Close()
		if test.expectedErr == "" {
			assert.Nil(t, err)
		} else {
			assert.EqualError(t, err, test.expectedErr)
		}
	}
}

type testCloser struct {
	err error
}

var _ io.Closer = (*testCloser)(nil)

func (t testCloser) Close() error {
	return t.err
}
