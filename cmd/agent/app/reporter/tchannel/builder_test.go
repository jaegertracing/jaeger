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

package tchannel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/uber/jaeger/pkg/discovery"
)

var yamlConfig = `
minPeers: 4

collectorHostPorts:
    - 127.0.0.1:14267
    - 127.0.0.1:14268
    - 127.0.0.1:14269

collectorServiceName: some-collector-service
`

func TestBuilderFromConfig(t *testing.T) {
	cfg := Builder{}
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err)

	assert.Equal(t, 4, cfg.DiscoveryMinPeers)
	assert.Equal(t, "some-collector-service", cfg.CollectorServiceName)
	assert.Equal(
		t,
		[]string{"127.0.0.1:14267", "127.0.0.1:14268", "127.0.0.1:14269"},
		cfg.CollectorHostPorts)
}

func TestBuilderWithDiscovery(t *testing.T) {
	cfg := &Builder{}
	discoverer := discovery.FixedDiscoverer([]string{"1.1.1.1:80"})
	cfg.WithDiscoverer(discoverer)
	_, err := cfg.CreateReporter(metrics.NullFactory, zap.NewNop())
	assert.EqualError(t, err, "cannot enable service discovery: both discovery.Discoverer and discovery.Notifier must be specified")

	cfg = &Builder{}
	notifier := &discovery.Dispatcher{}
	cfg.WithDiscoverer(discoverer).WithDiscoveryNotifier(notifier)
	agent, err := cfg.CreateReporter(metrics.NullFactory, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestBuilderWithCollectorServiceName(t *testing.T) {
	cfg := &Builder{}
	cfg.WithCollectorServiceName("svc")
	agent, err := cfg.CreateReporter(metrics.NullFactory, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, cfg.CollectorServiceName, "svc")

	cfg = NewBuilder()
	agent, err = cfg.CreateReporter(metrics.NullFactory, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, cfg.CollectorServiceName, "jaeger-collector")
}

func TestBuilderWithChannel(t *testing.T) {
	cfg := &Builder{}
	channel, _ := tchannel.NewChannel(agentServiceName, nil)
	cfg.WithChannel(channel)
	rep, err := cfg.CreateReporter(metrics.NullFactory, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, rep.Channel())
}

func TestBuilderWithCollectors(t *testing.T) {
	hostPorts := []string{"127.0.0.1:9876", "127.0.0.1:9877", "127.0.0.1:9878"}
	cfg := &Builder{
		CollectorHostPorts: hostPorts,
	}
	agent, err := cfg.CreateReporter(metrics.NullFactory, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)

	c, err := cfg.discoverer.Instances()
	assert.NoError(t, err)
	assert.Equal(t, c, hostPorts)
}
