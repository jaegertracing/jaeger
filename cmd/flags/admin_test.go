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

package flags

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestAdminServerHandlesPortZero(t *testing.T) {
	adminServer := NewAdminServer(":0")

	v, _ := config.Viperize(adminServer.AddFlags)

	zapCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(zapCore)

	adminServer.initFromViper(v, logger)

	assert.NoError(t, adminServer.Serve())
	defer adminServer.Close()

	message := logs.FilterMessage("Admin server started")
	assert.Equal(t, 1, message.Len(), "Expected Admin server started log message.")

	onlyEntry := message.All()[0]
	hostPort := onlyEntry.ContextMap()["http.host-port"].(string)
	port, _ := strconv.Atoi(strings.Split(hostPort, ":")[3])
	assert.Greater(t, port, 0)
}

func TestAdminServerParsesURL(t *testing.T) {
	testTable := []struct {
		input  string
		output string
		errstr string
	}{
		// success cases
		{":1337", "http://localhost:1337", ""},
		{"[2001:db8::1]:1337", "http://[2001:db8::1]:1337", ""},
		// failure cases
		{":13371337", "", "not a valid port"},
		{":0", "", "port is unknown"},
	}
	for _, test := range testTable {
		adm := NewAdminServer(test.input)
		url, err := adm.URL()
		if test.errstr == "" {
			assert.NoError(t, err)
			assert.Equal(t, test.output, url)
		} else {
			assert.EqualError(t, err, test.errstr)
		}
	}
}
