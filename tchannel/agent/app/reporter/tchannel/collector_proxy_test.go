// Copyright (c) 2018 The Jaeger Authors.
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

package tchannel

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/tchannel/agent/app/configmanager/tchannel"
)

var _ io.Closer = (*ProxyBuilder)(nil)

func TestErrorReporterBuilder(t *testing.T) {
	tbuilder := NewBuilder().WithDiscoverer(fakeDiscoverer{})
	b, err := NewCollectorProxy(tbuilder, metrics.NullFactory, zap.NewNop())
	require.Error(t, err)
	assert.Nil(t, b)
}

func TestCreate(t *testing.T) {
	cfg := &Builder{}
	mFactory := metrics.NullFactory
	logger := zap.NewNop()
	b, err := NewCollectorProxy(cfg, mFactory, logger)
	require.NoError(t, err)
	assert.NotNil(t, b)
	r, _ := cfg.CreateReporter(logger)
	assert.IsType(t, new(reporter.ClientMetricsReporter), b.GetReporter())
	m := tchannel.NewConfigManager(r.CollectorServiceName(), r.Channel())
	assert.Equal(t, configmanager.WrapWithMetrics(m, mFactory), b.GetManager())
	assert.Nil(t, b.Close())
}
