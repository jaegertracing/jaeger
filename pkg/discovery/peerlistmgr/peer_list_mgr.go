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
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

var introspectOptions = &tchannel.IntrospectionOptions{IncludeEmptyPeers: true}

// PeerListManager uses a discovery.Notifier to manage tchannel.PeerList
// by making sure that there are connections to at least minPeers.
type PeerListManager struct {
	peers      *tchannel.PeerList
	discoverer discovery.Discoverer // used for initial seed of the peers
	notifier   discovery.Notifier
	options
	rnd     *rand.Rand
	discoCh chan []string  // used to receive notifications
	stopCh  chan struct{}  // used to break out of timer loop that maintains connections
	exitWG  sync.WaitGroup // used to block Stop() until go-routines have exited
}

// New creates new PeerListManager.
func New(
	peerList *tchannel.PeerList,
	discoverer discovery.Discoverer,
	notifier discovery.Notifier,
	opts ...Option,
) (*PeerListManager, error) {
	options := Options.apply(opts...)
	mgr := &PeerListManager{
		options:    options,
		peers:      peerList,
		discoverer: discoverer,
		notifier:   notifier,
		discoCh:    make(chan []string, 100),
		stopCh:     make(chan struct{}),
		rnd:        rand.New(rand.NewSource(time.Now().UTC().UnixNano())),
	}

	instances, err := discoverer.Instances()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get initial set of instances")
	}
	mgr.updatePeers(instances)

	go mgr.processDiscoveryNotifications()
	go mgr.maintainConnections()

	notifier.Register(mgr.discoCh)
	return mgr, nil
}

// Stop shuts down the manager. It blocks until both bg go-routines exit.
func (m *PeerListManager) Stop() {
	m.notifier.Unregister(m.discoCh)
	m.exitWG.Add(2)
	close(m.discoCh)
	close(m.stopCh)
	m.exitWG.Wait()
}

func (m *PeerListManager) processDiscoveryNotifications() {
	defer m.exitWG.Done()
	for instances := range m.discoCh {
		m.updatePeers(instances)
	}
}

func (m *PeerListManager) maintainConnections() {
	defer m.exitWG.Done()

	ticker := time.NewTicker(m.connCheckFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.ensureConnections()
		case <-m.stopCh:
			return
		}
	}
}

func (m *PeerListManager) updatePeers(instances []string) {
	current := make(map[string]struct{})
	for _, addr := range instances {
		m.logger.Info("Registering active peer", zap.String("peer", addr))
		m.peers.Add(addr)
		current[addr] = struct{}{}
	}

	toRemove := []string{}
	for _, existing := range m.peers.IntrospectList(introspectOptions) {
		if _, ok := current[existing.HostPort]; !ok {
			toRemove = append(toRemove, existing.HostPort)
		}
	}
	for _, existing := range toRemove {
		m.logger.Info("Removing inactive peer", zap.String("peer", existing))
		m.peers.Remove(existing)
	}
}

func (m *PeerListManager) getMinPeers(peers map[string]*tchannel.Peer) int {
	minPeers := m.minPeers
	if l := len(peers); l < minPeers {
		minPeers = l
	}
	return minPeers
}

func (m *PeerListManager) findConnected(peers map[string]*tchannel.Peer) (connected int, noConn []*tchannel.Peer) {
	notConnected := make([]*tchannel.Peer, 0, len(peers))
	numConnected := 0
	for _, peer := range peers {
		_, out := peer.NumConnections()
		if out > 0 {
			numConnected++
		} else {
			notConnected = append(notConnected, peer)
		}
	}
	return numConnected, notConnected
}

func (m *PeerListManager) ensureConnections() {
	peers := m.peers.Copy()
	minPeers := m.getMinPeers(peers)
	numConnected, notConnected := m.findConnected(peers)
	if numConnected >= minPeers {
		return
	}
	m.logger.Info("Not enough connected peers",
		zap.Int("connected", numConnected),
		zap.Int("required", minPeers))
	for i := range notConnected {
		// swap current peer with random from the remaining positions
		r := i + m.rnd.Intn(len(notConnected)-i)
		notConnected[i], notConnected[r] = notConnected[r], notConnected[i]
		// try to connect to current peer (swapped)
		peer := notConnected[i]
		m.logger.Info("Trying to connect to peer", zap.String("host:port", peer.HostPort()))
		ctx, cancel := context.WithTimeout(context.Background(), m.connCheckTimeout)
		conn, err := peer.GetConnection(ctx)
		cancel()
		if err != nil {
			m.logger.Error("Unable to connect", zap.String("host:port", peer.HostPort()), zap.Duration("connCheckTimeout", m.connCheckTimeout), zap.Error(err))
			continue
		}

		if conn.IsActive() {
			m.logger.Info("Connected to peer", zap.String("host:port", conn.RemotePeerInfo().HostPort))
			numConnected++
			if numConnected >= minPeers {
				return
			}
		}
	}
}
