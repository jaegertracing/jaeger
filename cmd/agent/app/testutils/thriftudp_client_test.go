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

package testutils

import (
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewZipkinThriftUDPClient(t *testing.T) {
	_, _, err := NewZipkinThriftUDPClient("256.2.3:0")
	assert.Error(t, err)

	_, cl, err := NewZipkinThriftUDPClient("127.0.0.1:12345")
	require.NoError(t, err)
	cl.Close()
}

func TestNewJaegerThriftUDPClient(t *testing.T) {
	compactFactory := thrift.NewTCompactProtocolFactory()

	_, _, err := NewJaegerThriftUDPClient("256.2.3:0", compactFactory)
	assert.Error(t, err)

	_, cl, err := NewJaegerThriftUDPClient("127.0.0.1:12345", compactFactory)
	require.NoError(t, err)
	cl.Close()
}
