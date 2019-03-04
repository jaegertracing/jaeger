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
	"time"

	"github.com/pkg/errors"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/pkg/discovery/peerlistmgr"
)

const (
	defaultMinPeers = 3

	agentServiceName            = "jaeger-agent"
	defaultCollectorServiceName = "jaeger-collector"
	defaultReportTimeout        = time.Second
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

	// ConnCheckTimeout is the timeout used when establishing new connections.
	ConnCheckTimeout time.Duration

	// ReportTimeout is the timeout used when reporting span batches.
	ReportTimeout time.Duration

	discoverer discovery.Discoverer
	notifier   discovery.Notifier
	channel    *tchannel.Channel
}

// NewBuilder creates a new reporter builder.
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
		peerlistmgr.Options.Logger(logger),
		peerlistmgr.Options.ConnCheckTimeout(b.ConnCheckTimeout),
	)
}

// CreateReporter creates the TChannel-based Reporter
func (b *Builder) CreateReporter(agentTags map[string]string, logger *zap.Logger) (*Reporter, error) {
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

	if b.ReportTimeout == 0 {
		b.ReportTimeout = defaultReportTimeout
	}

	peerListMgr, err := b.enableDiscovery(b.channel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot enable service discovery")
	}
	return New(b.CollectorServiceName, b.channel, b.ReportTimeout, peerListMgr, agentTags, logger), nil
}

func defaultInt(value int, defaultVal int) int {
	if value == 0 {
		value = defaultVal
	}
	return value
}
