// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
