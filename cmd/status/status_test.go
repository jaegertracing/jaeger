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

package status

import (
	"testing"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/stretchr/testify/assert"
)

func TestStatusCommand(t *testing.T) {
	adminServer := flags.NewAdminServer(ports.PortToHostPort(2000))
	v, _ := config.Viperize(adminServer.AddFlags)

	cmd := Command(v, 2000)
	err := cmd.Execute()
	assert.EqualError(t, err, "no default admin port available for status")

	cmd.ParseFlags([]string{"--help"})
	err = cmd.Execute()
	assert.NoError(t, err)

	cmd.ParseFlags([]string{"--admin.http.host-port=1337"})
	err = cmd.Execute()
	assert.NoError(t, err)
	assert.NotNil(t, err)
}

// // Borrowed from: https://stackoverflow.com/a/26806093
// func captureOutput(f func()) string {
//     var buf bytes.Buffer
//     log.SetOutput(&buf)
//     f()
//     log.SetOutput(os.Stderr)
//     return buf.String()
// }
