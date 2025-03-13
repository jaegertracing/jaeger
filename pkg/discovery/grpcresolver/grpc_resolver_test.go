// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpcresolver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"

	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/pkg/discovery"
)

type testServer struct {
	grpc_testing.TestServiceServer
}

type test struct {
	servers   []*grpc.Server
	addresses []string
}

func (*testServer) EmptyCall(context.Context, *grpc_testing.Empty /* in */) (*grpc_testing.Empty, error) {
	return &grpc_testing.Empty{}, nil
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
		require.NoError(t, err, "failed to listen on tcp")
		s := grpc.NewServer()
		grpc_testing.RegisterTestServiceServer(s, &testServer{})
		testInstance.servers = append(testInstance.servers, s)
		testInstance.addresses = append(testInstance.addresses, lis.Addr().String())

		go func(s *grpc.Server, l net.Listener) {
			s.Serve(l)
		}(s, lis)
	}

	return testInstance
}

func makeSureConnectionsUp(t *testing.T, count int, testc grpc_testing.TestServiceClient) {
	var p peer.Peer
	addrs := make(map[string]struct{})
	// Make sure connections to all servers are up.
	for si := 0; si < count; si++ {
		connected := false
		for i := 0; i < 3000; i++ { // 3000 * 10ms = 30s
			if i != 0 {
				time.Sleep(time.Millisecond * 10)
			}
			_, err := testc.EmptyCall(context.Background(), &grpc_testing.Empty{}, grpc.Peer(&p))
			if err != nil {
				continue
			}
			if _, ok := addrs[p.Addr.String()]; !ok {
				addrs[p.Addr.String()] = struct{}{}
				connected = true
				t.Logf("connected to peer #%d (%v) on iteration %d", si, p.Addr, i)
				break
			}
		}
		assert.True(t, connected, "Connection #%d was still not up. Connections so far: %+v", si, addrs)
	}
}

func assertRoundRobinCall(t *testing.T, connections int, testc grpc_testing.TestServiceClient) {
	addrs := make(map[string]struct{})
	var p peer.Peer
	for i := 0; i < connections; i++ {
		_, err := testc.EmptyCall(context.Background(), &grpc_testing.Empty{}, grpc.Peer(&p))
		require.NoError(t, err)
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
	_, err := r.Build(resolver.Target{}, nil, resolver.BuildOptions{})
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
		{minPeers: 3, connections: 3},
		{minPeers: 5, connections: 3},
		// note: test cannot succeed with minPeers < connections because resolver
		// will never return more than minPeers addresses.
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%+v", test), func(t *testing.T) {
			res := New(notifier, discoverer, zap.NewNop(), test.minPeers)

			cc, err := grpc.NewClient(res.Scheme()+":///round_robin", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultServiceConfig(GRPCServiceConfig))
			require.NoError(t, err, "could not dial using resolver's scheme")
			defer cc.Close()

			testc := grpc_testing.NewTestServiceClient(cc)

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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
