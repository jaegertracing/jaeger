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
	"hash"
	"hash/fnv"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"

	"github.com/jaegertracing/jaeger/pkg/discovery"
)

// Resolver uses notifier to fetch list of available hosts
type Resolver struct {
	scheme            string
	cc                resolver.ClientConn
	notifier          discovery.Notifier
	discoverer        discovery.Discoverer
	logger            *zap.Logger
	discoCh           chan []string // used to receive notifications
	discoveryMinPeers int
	mu                sync.Mutex
	salt              []byte
	hasher            hash.Hash32

	//wg will wait for watcher to exit before firing .Close() to close resolver
	wg sync.WaitGroup
}
type hostScore struct {
	address string
	score   uint32
}

type hostScores []hostScore

func (s hostScores) Len() int           { return len(s) }
func (s hostScores) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s hostScores) Less(i, j int) bool { return s[i].score < s[j].score }

// New intialize a new grpc resolver with notifier
func New(
	notifier discovery.Notifier,
	discoverer discovery.Discoverer,
	logger *zap.Logger,
	discoveryMinPeers int,
) *Resolver {
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	r := &Resolver{
		notifier:          notifier,
		discoverer:        discoverer,
		discoCh:           make(chan []string, 100),
		logger:            logger,
		discoveryMinPeers: discoveryMinPeers,
		salt:              []byte(strconv.FormatInt(random.Int63(), 10)), // random salt for rendezvousHash
		scheme:            strconv.FormatInt(seed, 36),                   // make random scheme which will be used when registering
		hasher:            fnv.New32(),
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

	// Update conn states if proactively updates already work
	instances, err := r.discoverer.Instances()
	if err != nil {
		return nil, err
	}
	r.cc.UpdateState(resolver.State{Addresses: generateAddresses(instances)})
	r.wg.Add(1)
	go r.watcher()
	return r, nil
}

// Scheme returns the test scheme.
func (r *Resolver) Scheme() string {
	return r.scheme
}

// ResolveNow is a noop for Resolver.
func (r *Resolver) ResolveNow(o resolver.ResolveNowOption) {}

func (r *Resolver) watcher() {
	defer r.wg.Done()
	for latestHostPorts := range r.discoCh {
		r.mu.Lock()
		r.logger.Info("Received updates from notifier", zap.Strings("hostPorts", latestHostPorts))
		updatedAddresses := generateAddresses(r.rendezvousHash(latestHostPorts))
		r.cc.UpdateState(resolver.State{Addresses: updatedAddresses})
		r.mu.Unlock()
	}
}

// Close closes both discoCh
func (r *Resolver) Close() {
	r.notifier.Unregister(r.discoCh)
	close(r.discoCh)
	r.wg.Wait()
}

func (r *Resolver) rendezvousHash(addresses []string) []string {
	hosts := hostScores{}
	for _, address := range addresses {
		hosts = append(hosts, hostScore{
			address: address,
			score:   hashAddr(r.hasher, []byte(address), r.salt),
		})
	}
	sort.Sort(hosts)
	addressesPerHost := make([]string, r.discoveryMinPeers)
	for i := 0; i < r.discoveryMinPeers; i++ {
		addressesPerHost[i] = hosts[i].address
	}
	return addressesPerHost
}

func hashAddr(hasher hash.Hash32, node, saltKey []byte) uint32 {
	hasher.Reset()
	hasher.Write(saltKey)
	hasher.Write(node)
	return hasher.Sum32()
}

func generateAddresses(instances []string) []resolver.Address {
	var addrs []resolver.Address
	for _, instance := range instances {
		addrs = append(addrs, resolver.Address{Addr: instance})
	}
	return addrs
}
