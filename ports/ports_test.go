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

package ports

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortToHostPort(t *testing.T) {
	assert.Equal(t, ":42", PortToHostPort(42))
}

func TestGetAddressFromCLIOptionsLegacy(t *testing.T) {
	tests := []struct {
		port     int
		hostPort string
		out      string
	}{
		{port: 123, hostPort: "", out: ":123"},
		{port: 0, hostPort: "127.0.0.1:123", out: "127.0.0.1:123"},
		{port: 0, hostPort: ":123", out: ":123"},
		{port: 123, hostPort: "567", out: ":123"},
		{port: 0, hostPort: "123", out: ":123"},
		{port: 0, hostPort: "", out: ""},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%+v", test), func(t *testing.T) {
			assert.Equal(t, test.out, GetAddressFromCLIOptions(test.port, test.hostPort))
		})
	}
}
