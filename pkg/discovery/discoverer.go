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

package discovery

import "errors"

// Discoverer listens to a service discovery system and yields a set of
// identical instance locations. An error indicates a problem with connectivity
// to the service discovery system, or within the system itself; a subscriber
// may yield no endpoints without error.
type Discoverer interface {
	Instances() ([]string, error)
}

// FixedDiscoverer yields a fixed set of instances.
type FixedDiscoverer []string

// Instances implements Discoverer.
func (d FixedDiscoverer) Instances() ([]string, error) { return d, nil }

// ErrorDiscoverer yields a discoverer that returns error
type ErrorDiscoverer []string

// Instances implements Discoverer.
func (d ErrorDiscoverer) Instances() ([]string, error) {
	return nil, errors.New("error discoverer always return error")
}
