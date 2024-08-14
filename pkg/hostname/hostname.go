// Copyright (c) 2021 The Jaeger Authors.
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

package hostname

import (
	"crypto/rand"
	"fmt"
	"os"
	"sync"
)

type hostname struct {
	once     sync.Once
	err      error
	hostname string
}

var h hostname

// AsIdentifier uses the hostname of the os and postfixes a short random string to guarantee uniqueness
// The returned value is appropriate to use as a convenient unique identifier and will always be equal
// when called from within the same process.
func AsIdentifier() (string, error) {
	h.once.Do(func() {
		h.hostname, h.err = os.Hostname()
		if h.err != nil {
			return
		}

		buff := make([]byte, 8)
		_, h.err = rand.Read(buff)
		if h.err != nil {
			return
		}

		h.hostname = h.hostname + "-" + fmt.Sprintf("%2x", buff)
	})

	return h.hostname, h.err
}
