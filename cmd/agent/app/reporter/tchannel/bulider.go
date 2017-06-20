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
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/discovery"
	"github.com/uber/jaeger/pkg/discovery/peerlistmgr"
)

const (
	defaultMinPeers = 3

	agentServiceName            = "jaeger-agent"
	defaultCollectorServiceName = "jaeger-collector"
)

// Builder Struct to hold configurations
type Builder struct {
	// CollectorHostPorts are host:ports of a static list of Jaeger Collectors.
	CollectorHostPorts []string `yaml:"collectorHostPorts"`

	// MinPeers is the min number of servers we want the agent to connect to.
	// If zero, defaults to min(3, number of peers returned by service discovery)
	DiscoveryMinPeers int `yaml:"minPeers"`

	// CollectorServiceName is the name that Jaeger Collector's TChannel server
	// responds to.
	CollectorServiceName string `yaml:"collectorServiceName"`

	discoverer discovery.Discoverer
	notifier   discovery.Notifier
	channel    *tchannel.Channel
}

// NewBuilder creates a default builder with three processors.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithDiscoverer sets service discovery
func (b *Builder) WithDiscoverer(d discovery.Discoverer) *Builder {
	b.discoverer = d
	return b
}

// WithDiscoveryNotifier sets service discovery notifier
func (b *Builder) WithDiscoveryNotifier(n discovery.Notifier) *Builder {
	b.notifier = n
	return b
}

// WithCollectorServiceName sets collector service name
func (b *Builder) WithCollectorServiceName(s string) *Builder {
	b.CollectorServiceName = s
	return b
}

// WithChannel sets tchannel channel
func (b *Builder) WithChannel(c *tchannel.Channel) *Builder {
	b.channel = c
	return b
}

func (b *Builder) enableDiscovery(channel *tchannel.Channel, logger *zap.Logger) (*peerlistmgr.PeerListManager, error) {
	if b.discoverer == nil && b.notifier == nil {
		return nil, nil
	}
	if b.discoverer == nil || b.notifier == nil {
		return nil, errors.New("both discovery.Discoverer and discovery.Notifier must be specified")
	}

	logger.Info("Enabling service discovery", zap.String("service", b.CollectorServiceName))

	subCh := channel.GetSubChannel(b.CollectorServiceName, tchannel.Isolated)
	peers := subCh.Peers()
	return peerlistmgr.New(peers, b.discoverer, b.notifier,
		peerlistmgr.Options.MinPeers(defaultInt(b.DiscoveryMinPeers, defaultMinPeers)),
		peerlistmgr.Options.Logger(logger))
}

// CreateReporter creates the TChannel-based Reporter
func (b *Builder) CreateReporter(mFactory metrics.Factory, logger *zap.Logger) (*Reporter, error) {
	if b.channel == nil {
		// ignore errors since it only happens on empty service name
		b.channel, _ = tchannel.NewChannel(agentServiceName, nil)
	}

	if b.CollectorServiceName == "" {
		b.CollectorServiceName = defaultCollectorServiceName
	}

	// Use static collectors if specified.
	if len(b.CollectorHostPorts) != 0 {
		d := discovery.FixedDiscoverer(b.CollectorHostPorts)
		b = b.WithDiscoverer(d).WithDiscoveryNotifier(&discovery.Dispatcher{})
	}

	peerListMgr, err := b.enableDiscovery(b.channel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot enable service discovery")
	}
	return New(b.CollectorServiceName, b.channel, peerListMgr, mFactory, logger), nil
}

func defaultInt(value int, defaultVal int) int {
	if value == 0 {
		value = defaultVal
	}
	return value
}
