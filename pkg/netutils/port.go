// Copyright (c) 2020 The Jaeger Authors.
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

package netutils

import (
	"net"
	"strconv"
	"strings"
)

// GetPort returns the port of an endpoint address.
func GetPort(addr net.Addr) (int, error) {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return -1, err
	}

	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return -1, err
	}

	return parsedPort, nil
}

// FixLocalhost adds explicit localhost to endpoints binding to all interfaces because :port is not recognized by NO_PROXY
// e.g. :8080 becomes localhost:8080
func FixLocalhost(hostports []string) []string {
	var fixed []string
	for _, e := range hostports {
		if strings.HasPrefix(e, ":") {
			e = "localhost" + e
		}
		fixed = append(fixed, e)
	}
	return fixed
}
