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

package sanitizer

import (
	"github.com/uber/jaeger/model"
)

// SanitizeSpan sanitizes/normalizes spans. Any business logic that needs to be applied to normalize the contents of a
// span should implement this interface.
type SanitizeSpan func(span *model.Span) *model.Span

// NewChainedSanitizer creates a Sanitizer from the variadic list of passed Sanitizers
func NewChainedSanitizer(sanitizers ...SanitizeSpan) SanitizeSpan {
	return func(span *model.Span) *model.Span {
		for _, s := range sanitizers {
			span = s(span)

		}
		return span
	}
}
