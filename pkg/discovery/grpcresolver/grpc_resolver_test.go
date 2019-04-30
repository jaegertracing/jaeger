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
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	testpb "google.golang.org/grpc/test/grpc_testing"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

type testServer struct {
	testpb.TestServiceServer
}

type test struct {
	servers   []*grpc.Server
	addresses []string
}

func (s *testServer) EmptyCall(ctx context.Context, in *testpb.Empty) (*testpb.Empty, error) {
	return &testpb.Empty{}, nil
}

func (t *test) cleanup() {
	for _, s := range t.servers {
		s.Stop()
	}
}

func startTestServers(t *testing.T, count int) *test {
	testInstance := &test{}
	for i := 0; i < count; i++ {
		lis, err := net.Listen("tcp", "localhost:0")
		assert.NoError(t, err, "failed to listen on tcp")
		s := grpc.NewServer()
		testpb.RegisterTestServiceServer(s, &testServer{})
		testInstance.servers = append(testInstance.servers, s)
		testInstance.addresses = append(testInstance.addresses, lis.Addr().String())

		go func(s *grpc.Server, l net.Listener) {
			s.Serve(l)
		}(s, lis)
	}

	return testInstance
}

func TestErrorDiscoverer(t *testing.T) {
	notifier := &discovery.Dispatcher{}
	discoverer := discovery.ErrorDiscoverer{}
	r := New(notifier, discoverer, zap.NewNop(), 2)
	_, err := r.Build(resolver.Target{}, nil, resolver.BuildOption{})
	assert.Error(t, err)
}

func TestGRPCResolverRoundRobin(t *testing.T) {
	backendCount := 5

	test := startTestServers(t, backendCount)
	defer test.cleanup()

	notifier := &discovery.Dispatcher{}
	discoverer := discovery.FixedDiscoverer{}
	re := New(notifier, discoverer, zap.NewNop(), backendCount)

	cc, err := grpc.Dial(re.Scheme()+":///round_robin", grpc.WithInsecure(), grpc.WithBalancerName(roundrobin.Name))
	assert.NoError(t, err, "could not dial using resolver's scheme")
	defer cc.Close()
	testc := testpb.NewTestServiceClient(cc)

	notifier.Notify(test.addresses)

	var p peer.Peer
	// Make sure connections to all servers are up.
	for si := 0; si < backendCount; si++ {
		connected := false
		for i := 0; i < 100; i++ {
			_, err := testc.EmptyCall(context.Background(), &testpb.Empty{}, grpc.Peer(&p))
			assert.NoError(t, err)
			if p.Addr.String() == test.addresses[si] {
				connected = true
				break
			}
			time.Sleep(time.Millisecond)
		}
		assert.True(t, connected, "Connection was still not up")
	}

	addrs := make(map[string]struct{})
	for i := 0; i < backendCount; i++ {
		_, err := testc.EmptyCall(context.Background(), &testpb.Empty{}, grpc.Peer(&p))
		assert.NoError(t, err)
		addrs[p.Addr.String()] = struct{}{}
	}
	assert.Len(t, addrs, backendCount, "must call each of the servers once")
}

func TestRendezvousHashR(t *testing.T) {
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
