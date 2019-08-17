// Copyright (c) 2019 The Jaeger Authors.
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

package model

import (
	"hash/fnv"
	"io"
)

// Hashable interface is for type that can participate in a hash computation
// by writing their data into io.Writer, which is usually an instance of hash.Hash.
//
type Hashable interface {
	Hash(w io.Writer) error
}

// HashCode calculates a FNV-1a hash code for a Hashable object.
func HashCode(o Hashable) (uint64, error) {
	h := fnv.New64a()
	if err := o.Hash(h); err != nil {
		return 0, err
	}
	return h.Sum64(), nil
}
