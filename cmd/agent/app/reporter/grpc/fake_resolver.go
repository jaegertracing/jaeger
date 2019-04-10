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
	"sync"

	"google.golang.org/grpc/naming"
)

type fakeResolver struct {
	updateCh chan []string
}

type fakeWatcher struct {
	mu       sync.Mutex
	addrs    []string
	UpdateCh chan []string
}

func newFakeResolver(updateCh chan []string) *fakeResolver {
	return &fakeResolver{
		updateCh: updateCh,
	}
}

func (r *fakeResolver) Resolve(target string) (naming.Watcher, error) {
	w := fakeWatcher{
		UpdateCh: r.updateCh,
	}
	return &w, nil
}

func (w *fakeWatcher) Next() ([]*naming.Update, error) {
	latest := <-w.UpdateCh
	w.mu.Lock()
	defer w.mu.Unlock()
	var updates []*naming.Update
	for _, addr := range latest {
		if !contains(w.addrs, addr) {
			updates = append(updates, &naming.Update{Op: naming.Add, Addr: addr})
		}
	}
	for _, addr := range w.addrs {
		if !contains(latest, addr) {
			updates = append(updates, &naming.Update{Op: naming.Delete, Addr: addr})
		}
	}
	w.addrs = latest
	return updates, nil

}

func contains(updatedAddressList []string, address string) bool {
	for _, updatedAddress := range updatedAddressList {
		if updatedAddress == address {
			return true
		}
	}
	return false
}

func (w *fakeWatcher) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.UpdateCh != nil {
		close(w.UpdateCh)
		w.UpdateCh = nil
	}
}
