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

package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/naming"
)

func TestFakeResolver_Resolve(t *testing.T) {
	updateCh := make(chan []string)
	r := newFakeResolver(updateCh)
	watcher, err := r.Resolve("dns://test/test:default")
	defer watcher.Close()
	assert.NoError(t, err)

	go mockDNSUpdate(updateCh, []string{"111.222.333.444:8888", "111.222.333.445:8888"})
	// In reality, .Next() will be fired by roundrobin balancer to update list of available hosts
	// Here we call it manually for testing
	updates, err := watcher.Next()
	assert.NoError(t, err)

	assert.True(t, naming.Add == updates[0].Op && updates[0].Addr == "111.222.333.444:8888")
	assert.True(t, naming.Add == updates[1].Op && updates[1].Addr == "111.222.333.445:8888")

	go mockDNSUpdate(updateCh, []string{"111.222.333.446:8888", "111.222.333.445:8888"})
	updates, err = watcher.Next()
	assert.NoError(t, err)

	assert.True(t, naming.Add == updates[0].Op && updates[0].Addr == "111.222.333.446:8888")
	assert.True(t, naming.Delete == updates[1].Op && updates[1].Addr == "111.222.333.444:8888")
}

func mockDNSUpdate(updateCh chan []string, updates []string) {
	updateCh <- updates
}

func TestFakeResolver_RoundRobin(t *testing.T) {
	updateCh := make(chan []string)
	r := newFakeResolver(updateCh)
	numberGets := 1000
	address1 := "111.222.333.444:8888"
	address2 := "111.222.333.445:8888"
	b := grpc.RoundRobin(r)
	err := b.Start("dns://random", grpc.BalancerConfig{})
	assert.NoError(t, err)
	mockDNSUpdate(updateCh, []string{address1, address2})
	count := 0
	for i := 0; i < numberGets; i++ {
		address, _, _ := b.Get(context.Background(), grpc.BalancerGetOptions{
			BlockingWait: false,
		})
		if address.Addr == address1 {
			count += 1
		}
	}
	assert.Equal(t, 0.5, float64(count)/float64(numberGets), "Roundrobin balancer should return each address exactly half of the times")
}
