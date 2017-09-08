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

package distributedlock

import "time"

// Lock uses distributed lock for control of a resource.
type Lock interface {
	// Acquire acquires a lease of duration ttl around a given resource. In case of an error,
	// acquired is meaningless.
	Acquire(resource string, ttl time.Duration) (acquired bool, err error)

	Forfeit(resource string) (forfeited bool, err error)
}
