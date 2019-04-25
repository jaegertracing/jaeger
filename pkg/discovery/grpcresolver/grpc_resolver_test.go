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
	"hash/fnv"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc/resolver"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/peer"
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

func startTestServers(count int) (_ *test, err error) {
	t := &test{}

	defer func() {
		if err != nil {
			t.cleanup()
		}
	}()
	for i := 0; i < count; i++ {
		lis, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			return nil, fmt.Errorf("failed to listen %v", err)
		}

		s := grpc.NewServer()
		testpb.RegisterTestServiceServer(s, &testServer{})
		t.servers = append(t.servers, s)
		t.addresses = append(t.addresses, lis.Addr().String())

		go func(s *grpc.Server, l net.Listener) {
			s.Serve(l)
		}(s, lis)
	}

	return t, nil
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

	test, err := startTestServers(backendCount)
	if err != nil {
		t.Fatalf("failed to start servers: %v", err)
	}
	defer test.cleanup()

	notifier := &discovery.Dispatcher{}
	//discoverer := discovery.FixedDiscoverer(test.addresses)
	discoverer := discovery.FixedDiscoverer{}
	re := New(notifier, discoverer, zap.NewNop(), backendCount)
	assert.NoError(t, err)

	cc, err := grpc.Dial(re.Scheme()+":///round_robin", grpc.WithInsecure(), grpc.WithBalancerName(roundrobin.Name))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer cc.Close()
	testc := testpb.NewTestServiceClient(cc)

	notifier.Notify(test.addresses)

	var p peer.Peer
	// Make sure connections to all servers are up.
	for si := 0; si < backendCount; si++ {
		var connected bool
		for i := 0; i < 100; i++ {
			if _, err := testc.EmptyCall(context.Background(), &testpb.Empty{}, grpc.Peer(&p)); err != nil {
				t.Fatalf("EmptyCall() = _, %v, want _, <nil>", err)
			}
			if p.Addr.String() == test.addresses[si] {
				connected = true
				break
			}
			time.Sleep(time.Millisecond)
		}
		if !connected {
			t.Fatalf("Connection to %v was still not up", test.addresses[si])
		}
	}

	if _, err := testc.EmptyCall(context.Background(), &testpb.Empty{}, grpc.Peer(&p)); err != nil {
		t.Fatalf("EmptyCall() = _, %v, want _, <nil>", err)
	}
	previousAddr := p
	firstAddr := p
	for i := 0; i < backendCount; i++ {
		if _, err := testc.EmptyCall(context.Background(), &testpb.Empty{}, grpc.Peer(&p)); err != nil {
			t.Fatalf("EmptyCall() = _, %v, want _, <nil>", err)
		}
		if previousAddr == p {
			t.Fatal("Roundrobin balancer shouldn't call same host/port in a row")
		}
		previousAddr = p
	}
	if firstAddr != p {
		t.Fatal("After a full cycle the first host/port should be called again")
	}
}

func TestRendezvousHashR(t *testing.T) {
	// Rendezvous Hash should return same subset with same addresses & salt string
	addresses := []string{"127.1.0.3:8080", "127.0.1.1:8080", "127.2.1.2:8080", "127.3.0.4:8080"}
	sameAddressesDifferentOrder := []string{"127.2.1.2:8080", "127.1.0.3:8080", "127.3.0.4:8080", "127.0.1.1:8080"}
	hasher := fnv.New32()
	saltByte := []byte("example-salt")
	subset1 := rendezvousHash(addresses, saltByte, hasher, 2)
	subset2 := rendezvousHash(sameAddressesDifferentOrder, saltByte, hasher, 2)
	assert.Equal(t, subset1, subset2)
}
