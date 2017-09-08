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
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tchannel "github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
	"go.uber.org/zap"
	"golang.org/x/net/context"

	"github.com/uber/jaeger/pkg/discovery"
)

type testManager struct {
	subChannel *tchannel.SubChannel
	discoverer discovery.Discoverer
	notifier   *discovery.Dispatcher
	mgr        *PeerListManager
}

func withTestManager(t *testing.T, f func(tm *testManager), opts ...Option) {
	ch, err := tchannel.NewChannel("test-client", nil)
	require.NoError(t, err)
	subCh := ch.GetSubChannel("test-server", tchannel.Isolated)

	discoverer := discovery.FixedDiscoverer([]string{})
	notifier := &discovery.Dispatcher{}

	peerMgr, err := New(subCh.Peers(), discoverer, notifier, opts...)
	require.NoError(t, err)
	defer peerMgr.Stop()

	f(&testManager{
		subChannel: subCh,
		discoverer: discoverer,
		notifier:   notifier,
		mgr:        peerMgr,
	})
}

type badDiscoverer struct{}

func (b badDiscoverer) Instances() ([]string, error) {
	return nil, errors.New("bad discoverer")
}

func TestPeerListManager_NoInstances(t *testing.T) {
	_, err := New(&tchannel.PeerList{}, badDiscoverer{}, &discovery.Dispatcher{})
	assert.EqualError(t, err, "cannot get initial set of instances: bad discoverer")
}

func TestPeerListManager_updatePeers(t *testing.T) {
	withTestManager(t, func(tm *testManager) {
		mkInstance := func(ip byte) string {
			return fmt.Sprintf("0.0.0.%d:12345", ip)
		}
		mkInstances := func(ips ...byte) []string {
			instances := make([]string, len(ips))
			for i := range ips {
				instances[i] = mkInstance(ips[i])
			}
			return instances
		}

		testCases := []struct {
			instances []string
			name      string
		}{
			{instances: mkInstances(1, 2), name: "initial"},
			{instances: mkInstances(1, 2, 3), name: "add one"},
			{instances: mkInstances(2, 3), name: "remove one"},
			{instances: mkInstances(4), name: "replace all"},
		}

		countPeers := func() int {
			peers := tm.subChannel.Peers().IntrospectList(introspectOptions)
			return len(peers)
		}

		for _, tc := range testCases {
			testCase := tc // capture loop var
			t.Run(testCase.name, func(t *testing.T) {

				countBefore := countPeers()
				tm.notifier.Notify(testCase.instances)

				// the peer manager receives notifications via channel, so yield to let it process
				for i := 0; i < 1000; i++ {
					if countPeers() != countBefore {
						break
					}
					time.Sleep(time.Millisecond) // wait up to a second
				}

				peers := tm.subChannel.Peers().IntrospectList(introspectOptions)
				assert.Equal(t, len(testCase.instances), len(peers))
				for _, peer := range peers {
					found := false
					for _, instance := range testCase.instances {
						if peer.HostPort == instance {
							found = true
							break
						}
					}
					assert.True(t, found, "expecting to find peer %+v", peer)
				}
			})

		}

	}, Options.MinPeers(3))

}

func TestPeerListManager_getMinPeers(t *testing.T) {
	withTestManager(t, func(tm *testManager) {
		n1 := tm.mgr.getMinPeers(map[string]*tchannel.Peer{
			"a": {},
		})
		assert.Equal(t, 1, n1)
		n2 := tm.mgr.getMinPeers(map[string]*tchannel.Peer{
			"a": {},
			"b": {},
			"c": {},
		})
		assert.Equal(t, 2, n2)
	}, Options.MinPeers(2))
}

func TestPeerListManager_ensureConnection(t *testing.T) {
	testCases := []struct {
		numServers    int // how many instances of the service to stand up
		numSeeds      int // how many of them to tell the peer manager about
		minPeers      int // how many peer connections the mgr should maintain open
		numConnected1 int // how many connected peers we expect after the first ensureConnection
	}{
		{numServers: 3, numSeeds: 1, minPeers: 2, numConnected1: 1},
		{numServers: 3, numSeeds: 2, minPeers: 2, numConnected1: 2},
		{numServers: 3, numSeeds: 3, minPeers: 2, numConnected1: 2},
	}

	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(fmt.Sprintf("%+v", testCase), func(t *testing.T) {
			// TODO zaptest.Buffer appears to be not thread-safe https://github.com/uber-go/zap/issues/399
			//logger, log := testlog.NewLogger()
			logger := zap.NewNop()
			log := &bytes.Buffer{}

			var servers []*tchannel.Channel
			for i := 0; i < testCase.numServers; i++ {
				ch, closer := makeTestTChannelServer(t, i)
				defer closer()
				servers = append(servers, ch)
			}

			withTestManager(t,
				func(tm *testManager) {
					var instances []string
					for i := 0; i < testCase.numSeeds; i++ {
						instances = append(instances, servers[i].PeerInfo().HostPort)
					}
					tm.mgr.updatePeers(instances)

					if !assertConnections(t, tm.mgr, testCase.numConnected1) {
						t.Fatal(log.String())
					}

					id := callServer(t, tm.subChannel)
					serverAddr := servers[id].PeerInfo().HostPort
					logger.Info("Stopping server", zap.Int("id", id), zap.String("addr", serverAddr))
					servers[id].Close()

					if !assertDisconnected(t, tm.mgr, serverAddr) {
						t.Fatal(log.String())
					}

					if testCase.minPeers >= testCase.numSeeds {
						// test case didn't provide enough seeds to maintain minPeers after disconnect,
						// so give the peer manager all known servers (including the closed one)
						var instances []string
						for i := 0; i < testCase.numServers; i++ {
							instances = append(instances, servers[i].PeerInfo().HostPort)
						}
						tm.mgr.updatePeers(instances)
					}

					if !assertConnections(t, tm.mgr, testCase.minPeers) {
						t.Fatal(log.String())
					}
				},
				Options.MinPeers(testCase.minPeers),
				Options.ConnCheckFrequency(time.Millisecond),
				Options.Logger(logger))
		})
	}
}

// assertConnections verifies that peer list has expectedConns number of connected peers.
func assertConnections(t *testing.T, mgr *PeerListManager, expectedConns int) bool {
	for i := 0; i < 1000; i++ { // wait up to 1s
		connected, _ := mgr.findConnected(mgr.peers.Copy())
		if connected == expectedConns {
			break
		}
		time.Sleep(time.Millisecond)
	}

	connected, _ := mgr.findConnected(mgr.peers.Copy())
	return assert.Equal(t, expectedConns, connected, "expecting certain number of open connections")
}

// assertDisconnected verifies that the serverAddr is either removed from the list of peers
// or has zero outbound connections.
func assertDisconnected(t *testing.T, mgr *PeerListManager, serverAddr string) bool {
	start := time.Now()
	disconnected := false
	for i := 0; i < 1000; i++ { // wait up to 1s to disconnect
		peers := mgr.peers.Copy()
		if p, ok := peers[serverAddr]; ok {
			if _, out := p.NumConnections(); out == 0 {
				disconnected = true
				break
			}
		} else {
			disconnected = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	return assert.True(t, disconnected,
		"expecting peer %s to disconnect or be removed from the list, wait time %s",
		serverAddr, time.Since(start).String())
}

// callServer calls a random peer on the subChannel.
// The return value is the id of the server that responded.
func callServer(t *testing.T, subCh *tchannel.SubChannel) int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := raw.CallV2(ctx, subCh, raw.CArgs{Method: "test-method"})
	require.NoError(t, err)
	require.Len(t, res.Arg3, 1)
	return int(res.Arg3[0])
}

// makeTestTChannelServer creates a server that returns its own ID as response to any call
func makeTestTChannelServer(t *testing.T, id int) (ch *tchannel.Channel, closer func()) {
	ch, err := testutils.NewServerChannel(&testutils.ChannelOpts{
		ServiceName: "test-server",
	})
	require.NoError(t, err)
	ch.Register(raw.Wrap(&rawHandler{t: t, id: id}), "test-method")
	closer = func() {
		if !ch.Closed() {
			ch.Close()
		}
	}
	return
}

type rawHandler struct {
	t  *testing.T
	id int
}

func (h *rawHandler) Handle(ctx context.Context, args *raw.Args) (*raw.Res, error) {
	return &raw.Res{Arg3: []byte{byte(h.id)}}, nil
}

func (h *rawHandler) OnError(ctx context.Context, err error) { h.t.Errorf("onError %v", err) }
