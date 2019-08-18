// Copyright (c) 2019 The Jaeger Authors.
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

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

func TestAllOptionSet(t *testing.T) {
	types := []SpanFormat{SpanFormat("sneh")}
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
