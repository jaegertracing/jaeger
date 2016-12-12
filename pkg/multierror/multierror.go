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
	"fmt"
	"strings"
)

// Wrap takes an array of errors and returns a single error that encapsulates
// those underlying errors. If the array is nil or empty it returns nil.
// If the array only contains a single element, that error is returned directly.
func Wrap(errs []error) error {
	return multiError(errs).AsError()
}

// Errors bundles more than one error together into a single error.
type multiError []error

// AsError returns either: nil, the only error, or the Error instance itself
// if there are 0, 1, or more errors in the slice respectively.  This method is
// useful for contexts that want to do a simple return Errors(errors).AsError().
func (errors multiError) AsError() error {
	switch len(errors) {
	case 0:
		return nil
	case 1:
		return errors[0]
	default:
		return errors
	}
}

// Error returns a string like "[e1, e2, ...]" where each eN is the Error() of
// each error in the slice.
func (errors multiError) Error() string {
	parts := make([]string, len(errors))
	for i, err := range errors {
		parts[i] = err.Error()
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}
