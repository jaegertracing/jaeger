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

package tchannel

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/pkg/discovery"
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
	r, err := cfg.CreateReporter(nil, zap.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "some-collector-service", r.CollectorServiceName())

}

func TestBuilderWithDiscovery(t *testing.T) {
	cfg := &Builder{}
	discoverer := discovery.FixedDiscoverer([]string{"1.1.1.1:80"})
	cfg.WithDiscoverer(discoverer)
	_, err := cfg.CreateReporter(nil, zap.NewNop())
	assert.EqualError(t, err, "cannot enable service discovery: both discovery.Discoverer and discovery.Notifier must be specified")

	cfg = &Builder{}
	notifier := &discovery.Dispatcher{}
	cfg.WithDiscoverer(discoverer).WithDiscoveryNotifier(notifier)
	agent, err := cfg.CreateReporter(nil, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestBuilderWithDiscoveryError(t *testing.T) {
	tbuilder := NewBuilder().WithDiscoverer(fakeDiscoverer{})
	rep, err := tbuilder.CreateReporter(nil, zap.NewNop())
	assert.EqualError(t, err, "cannot enable service discovery: both discovery.Discoverer and discovery.Notifier must be specified")
	assert.Nil(t, rep)
}

func TestBuilderWithCollectorServiceName(t *testing.T) {
	cfg := &Builder{}
	cfg.WithCollectorServiceName("svc")
	agent, err := cfg.CreateReporter(nil, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, cfg.CollectorServiceName, "svc")

	cfg = NewBuilder()
	agent, err = cfg.CreateReporter(nil, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, cfg.CollectorServiceName, "jaeger-collector")
}

func TestBuilderWithChannel(t *testing.T) {
	cfg := &Builder{}
	channel, _ := tchannel.NewChannel(agentServiceName, nil)
	cfg.WithChannel(channel)
	rep, err := cfg.CreateReporter(nil, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, rep.Channel())
	assert.Equal(t, defaultCollectorServiceName, rep.CollectorServiceName())
}

func TestBuilderWithCollectors(t *testing.T) {
	hostPorts := []string{"127.0.0.1:9876", "127.0.0.1:9877", "127.0.0.1:9878"}
	cfg := &Builder{
		CollectorHostPorts: hostPorts,
	}
	agent, err := cfg.CreateReporter(nil, zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)

	c, err := cfg.discoverer.Instances()
	assert.NoError(t, err)
	assert.Equal(t, c, hostPorts)
}

type fakeDiscoverer struct{}

func (fd fakeDiscoverer) Instances() ([]string, error) {
	return nil, errors.New("discoverer error")
}
