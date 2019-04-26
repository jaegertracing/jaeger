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

package app

import (
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
)

func TestServer(t *testing.T) {
	const testPort = ports.QueryAdminHTTP

	flagsSvc := flags.NewService(ports.AgentAdminHTTP)
	flagsSvc.HC().Ready()
	err := flagsSvc.Admin.Serve()
	assert.NoError(t, err)
	flagsSvc.Logger = zap.NewNop()
	go flagsSvc.RunAndThen(func() {
		// no op
	})

	router := mux.NewRouter()
	querySvc := querysvc.QueryService{}
	tracker := opentracing.NoopTracer{}

	server, err := NewServer(flagsSvc, router,querySvc, tracker, testPort)
	assert.NoError(t, err)

	server.Start()

	err = server.httpServer.Close()
	assert.NoError(t, err)
	// wait before server is closed
	time.Sleep(1 * time.Second)

	// after shutdown is called, status gets changed to Broken
	assert.Equal(t, healthcheck.Broken, server.svc.HC().Get())
}
