// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://wwr.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpcresolver

import (
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/tysontate/rendezvous"
	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

// Resolver uses notifier to fetch list of available hosts
type Resolver struct {
	scheme string

	// Fields actually belong to the resolver.
	cc resolver.ClientConn

	notifier   discovery.Notifier
	logger     *zap.Logger
	discoCh    chan []string // used to receive notifications
	stopCh     chan struct{}
	subsetSize int
	mu         sync.Mutex
	hash       rendezvous.Hash
	salt       string
}

// New intialize a new grpc resolver with notifier
func New(
	notifier discovery.Notifier,
	logger *zap.Logger,
	subsetSize int,
) *Resolver {
	rand.Seed(time.Now().UTC().UnixNano())

	r := &Resolver{
		notifier:   notifier,
		discoCh:    make(chan []string, 100), // TODO should this number be configurable? What if the number of collectors exceed 100 from discovery service
		stopCh:     make(chan struct{}),
		logger:     logger,
		subsetSize: subsetSize,
		salt:       strconv.FormatInt(rand.Int63(), 10),
		scheme:     strconv.FormatInt(time.Now().UnixNano(), 36),
	}
	// TODO not sure if there's an equivalent way for grpc to maintain connection like what tchannel did?

	// Register the resolver with grpc so it's available for grpc.Dial
	resolver.Register(r)

	// Register the discoCh channel with notifier so it continues to fetch a list of host/port
	notifier.Register(r.discoCh)
	return r
}

// Build returns itself for Resolver, because it's both a builder and a resolver.
func (r *Resolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOption) (resolver.Resolver, error) {
	r.cc = cc
	go r.watcher()
	return r, nil
}

// Scheme returns the test scheme.
func (r *Resolver) Scheme() string {
	return r.scheme
}

// ResolveNow is a noop for Resolver.
func (*Resolver) ResolveNow(o resolver.ResolveNowOption) {}

func (r *Resolver) watcher() {
	for {
		select {
		case latestHostPorts := <-r.discoCh:
			r.mu.Lock()
			defer r.mu.Unlock()
			r.logger.Info("gRPC naming.Watcher Receives updates", zap.Strings("hostPorts", latestHostPorts))
			subsetHostPorts := rendezvousHash(latestHostPorts, r.salt, r.subsetSize)
			var resolvedAddrs []resolver.Address
			for _, addr := range subsetHostPorts {
				resolvedAddrs = append(resolvedAddrs, resolver.Address{Addr: addr})
			}
			r.cc.UpdateState(resolver.State{Addresses: resolvedAddrs})
		case <-r.stopCh:
			return
		}
	}
}

// Close closes both discoCh and stopCh
func (r *Resolver) Close() {
	if r.discoCh != nil {
		r.notifier.Unregister(r.discoCh)
		close(r.discoCh)
		r.discoCh = nil
	}

	if r.stopCh != nil {
		close(r.stopCh)
		r.stopCh = nil
	}
}

func rendezvousHash(addresses []string, salt string, subsetSize int) []string {
	hash := rendezvous.New(addresses...)
	subset := hash.GetN(subsetSize, salt)
	return subset
}
