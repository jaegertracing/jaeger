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

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/model"
)

func TestAllOptionSet(t *testing.T) {
	types := []string{"sneh"}
	opts := Options.apply(
		Options.ReportBusy(true),
		Options.BlockingSubmit(true),
		Options.ExtraFormatTypes(types),
		Options.SpanFilter(func(span *model.Span) bool { return true }),
		Options.HostMetrics(metrics.NullFactory),
		Options.ServiceMetrics(metrics.NullFactory),
		Options.Logger(zap.NewNop()),
		Options.NumWorkers(5),
		Options.PreProcessSpans(func(spans []*model.Span) {}),
		Options.Sanitizer(func(span *model.Span) *model.Span { return span }),
		Options.QueueSize(10),
		Options.PreSave(func(span *model.Span) {}),
	)
	assert.EqualValues(t, 5, opts.numWorkers)
	assert.EqualValues(t, 10, opts.queueSize)
}

func TestNoOptionsSet(t *testing.T) {
	opts := Options.apply()
	assert.EqualValues(t, DefaultNumWorkers, opts.numWorkers)
	assert.EqualValues(t, 0, opts.queueSize)
	assert.False(t, opts.reportBusy)
	assert.False(t, opts.blockingSubmit)
	assert.NotPanics(t, func() { opts.preProcessSpans(nil) })
	assert.NotPanics(t, func() { opts.preSave(nil) })
	assert.True(t, opts.spanFilter(nil))
	span := model.Span{}
	assert.EqualValues(t, &span, opts.sanitizer(&span))
}
