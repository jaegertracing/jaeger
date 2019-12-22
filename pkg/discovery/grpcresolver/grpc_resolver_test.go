// Copyright (c) 2019 The Jaeger Authors.
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

package grpcresolver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	grpctest "google.golang.org/grpc/test/grpc_testing"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

type testServer struct {
	grpctest.TestServiceServer
}

type test struct {
	servers   []*grpc.Server
	addresses []string
}

func (s *testServer) EmptyCall(ctx context.Context, in *grpctest.Empty) (*grpctest.Empty, error) {
	return &grpctest.Empty{}, nil
}

func (t *test) cleanup() {
	for _, s := range t.servers {
		s.Stop()
	}
}

type erroredDiscoverer struct {
	err error
}

// Instances implements Discoverer.
func (d erroredDiscoverer) Instances() ([]string, error) {
	return nil, d.err
}

func startTestServers(t *testing.T, count int) *test {
	testInstance := &test{}
	for i := 0; i < count; i++ {
		lis, err := net.Listen("tcp", "localhost:0")
		assert.NoError(t, err, "failed to listen on tcp")
		s := grpc.NewServer()
		grpctest.RegisterTestServiceServer(s, &testServer{})
		testInstance.servers = append(testInstance.servers, s)
		testInstance.addresses = append(testInstance.addresses, lis.Addr().String())

		go func(s *grpc.Server, l net.Listener) {
			s.Serve(l)
		}(s, lis)
	}

	return testInstance
}

func makeSureConnectionsUp(t *testing.T, count int, testc grpctest.TestServiceClient) {
	var p peer.Peer
	addrs := make(map[string]struct{})
	// Make sure connections to all servers are up.
	for si := 0; si < count; si++ {
		connected := false
		for i := 0; i < 1000; i++ {
			_, err := testc.EmptyCall(context.Background(), &grpctest.Empty{}, grpc.Peer(&p))
			if err != nil {
				continue
			}
			if _, ok := addrs[p.Addr.String()]; !ok {
				addrs[p.Addr.String()] = struct{}{}
				connected = true
				break
			}
			time.Sleep(time.Millisecond * 10)
		}
		assert.True(t, connected, "Connection was still not up")
	}
}

func assertRoundRobinCall(t *testing.T, connections int, testc grpctest.TestServiceClient) {
	addrs := make(map[string]struct{})
	var p peer.Peer
	for i := 0; i < connections; i++ {
		_, err := testc.EmptyCall(context.Background(), &grpctest.Empty{}, grpc.Peer(&p))
		assert.NoError(t, err)
		addrs[p.Addr.String()] = struct{}{}
	}
	assert.Len(t, addrs, connections, "must call each of the servers once")
}

func TestErrorDiscoverer(t *testing.T) {
	notifier := &discovery.Dispatcher{}
	errMessage := errors.New("error discoverer returns error")
	discoverer := erroredDiscoverer{
		err: errMessage,
	}
	r := New(notifier, discoverer, zap.NewNop(), 2)
	_, err := r.Build(resolver.Target{}, nil, resolver.BuildOption{})
	assert.Equal(t, errMessage, err)
}

func TestGRPCResolverRoundRobin(t *testing.T) {
	backendCount := 5

	testInstances := startTestServers(t, backendCount)
	defer testInstances.cleanup()

	notifier := &discovery.Dispatcher{}
	discoverer := discovery.FixedDiscoverer{}

	tests := []struct {
		minPeers    int
		connections int // expected number of unique connections to servers
	}{
		{3, 3}, {5, 5}, {7, 5},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("minPeers=%d", test.minPeers), func(t *testing.T) {
			res := New(notifier, discoverer, zap.NewNop(), test.minPeers)
			defer resolver.UnregisterForTesting(res.Scheme())

			cc, err := grpc.Dial(res.Scheme()+":///round_robin", grpc.WithInsecure(), grpc.WithDefaultServiceConfig(GRPCServiceConfig))
			assert.NoError(t, err, "could not dial using resolver's scheme")
			defer cc.Close()

			testc := grpctest.NewTestServiceClient(cc)

			notifier.Notify(testInstances.addresses)

			// This step is necessary to ensure that connections to all min-peers are ready,
			// otherwise round-robin may loop only through already connected peers.
			makeSureConnectionsUp(t, test.connections, testc)

			assertRoundRobinCall(t, test.connections, testc)
		})
	}
}

func TestRendezvousHash(t *testing.T) {
	// Rendezvous Hash should return same subset with same addresses & salt string
	addresses := []string{"127.1.0.3:8080", "127.0.1.1:8080", "127.2.1.2:8080", "127.3.0.4:8080"}
	sameAddressesDifferentOrder := []string{"127.2.1.2:8080", "127.1.0.3:8080", "127.3.0.4:8080", "127.0.1.1:8080"}
	notifier := &discovery.Dispatcher{}
	discoverer := discovery.FixedDiscoverer{}
	resolverInstance := New(notifier, discoverer, zap.NewNop(), 2)
	subset1 := resolverInstance.rendezvousHash(addresses)
	subset2 := resolverInstance.rendezvousHash(sameAddressesDifferentOrder)
	assert.Equal(t, subset1, subset2)
}
