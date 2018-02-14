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

package peerlistmgr

import (
	"math/rand"
	"time"

	"github.com/go-kit/kit/sd"
	"github.com/pkg/errors"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

// PeerListManagerV2 uses a go-kit sd.Instancer to manage tchannel.PeerList
// by making sure that there are connections to at least minPeers.
type PeerListManagerV2 struct {
	PeerListManager
	instancer sd.Instancer
	discoCh   chan sd.Event
}

// NewV2 creates new PeerListManagerV2.
func NewV2(
	peerList *tchannel.PeerList,
	discoverer discovery.Discoverer,
	instancer sd.Instancer,
	opts ...Option,
) (*PeerListManagerV2, error) {
	options := Options.apply(opts...)
	mgr := &PeerListManagerV2{
		PeerListManager: PeerListManager{
			options:    options,
			peers:      peerList,
			discoverer: discoverer,
			stopCh:     make(chan struct{}),
			rnd:        rand.New(rand.NewSource(time.Now().UTC().UnixNano())),
		},
		discoCh:   make(chan sd.Event, 100),
		instancer: instancer,
	}

	instances, err := discoverer.Instances()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get initial set of instances")
	}
	mgr.updatePeers(sd.Event{Instances: instances})

	go mgr.processDiscoveryNotifications()
	go mgr.maintainConnections()

	instancer.Register(mgr.discoCh)
	return mgr, nil
}

// Stop shuts down the manager. It blocks until both bg go-routines exit.
func (m *PeerListManagerV2) Stop() {
	m.instancer.Deregister(m.discoCh)
	m.exitWG.Add(2)
	close(m.discoCh)
	close(m.stopCh)
	m.exitWG.Wait()
}

func (m *PeerListManagerV2) processDiscoveryNotifications() {
	defer m.exitWG.Done()
	for event := range m.discoCh {
		m.updatePeers(event)
	}
}

func (m *PeerListManagerV2) updatePeers(event sd.Event) {
	if event.Err != nil {
		m.logger.Error("Received error event", zap.Error(event.Err))
		// TODO delete all existing peers?
		return
	}
	m.PeerListManager.updatePeers(event.Instances)
}
