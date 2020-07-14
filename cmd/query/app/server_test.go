// Copyright (c) 2019,2020 The Jaeger Authors.
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
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestServerError(t *testing.T) {
	srv := &Server{
		queryOptions: &QueryOptions{
			HostPort: ":-1",
		},
	}
	assert.Error(t, srv.Start())
}

func TestCreateTLSGrpcServerError(t *testing.T) {
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}

	_, err := NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{TLSGRPC: tlsCfg}, opentracing.NoopTracer{})
	assert.NotNil(t, err)
}

func TestCreateTLSHttpServerError(t *testing.T) {
	tlsCfg := tlscfg.Options{
		Enabled:      true,
		CertPath:     "invalid/path",
		KeyPath:      "invalid/path",
		ClientCAPath: "invalid/path",
	}

	_, err := NewServer(zap.NewNop(), &querysvc.QueryService{},
		&QueryOptions{TLSHTTP: tlsCfg}, opentracing.NoopTracer{})
	assert.NotNil(t, err)
}

func TestTLSHTTPServer(t *testing.T) {
	tests := []struct {
		name             string
		serverTLS        tlscfg.Options
		clientTLS        tlscfg.Options
		expectError      bool
		expectServerFail bool
	}{
		// {
		// 	name: "",
		// 	serverTLS: tlscfg.Options{
		// 		Enabled:      true,
		// 		CertPath:     "invalid/path",
		// 		KeyPath:      "invalid/path",
		// 		ClientCAPath: "invalid/path",
		// 	},
		// 	clientTLS: tlscfg.Options{
		// 		Enabled:    true,
		// 		CertPath:   "invalid/path",
		// 		KeyPath:    "invalid/path",
		// 		CAPath:     "invalid/path",
		// 		ServerName: "example.com",
		// 	},
		// 	expectError:      false,
		// 	expectServerFail: false,
		// },
		{
			name: "Should pass with insecure connection",
			serverTLS: tlscfg.Options{
				Enabled: false,
			},
			clientTLS: tlscfg.Options{
				Enabled: false,
			},
			expectError:      false,
			expectServerFail: false,
		},
		{
			name: "should fail with TLS client to untrusted TLS server",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: "testdata/example-server-cert.pem",
				KeyPath:  "testdata/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				ServerName: "example.com",
			},
			expectError:      true,
			expectServerFail: false,
		},
		{
			name: "should fail with TLS client to trusted TLS server with incorrect hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: "testdata/example-server-cert.pem",
				KeyPath:  "testdata/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/example-CA-cert.pem",
				ServerName: "nonEmpty",
			},
			expectError:      true,
			expectServerFail: false,
		},
		{
			name: "should pass with TLS client to trusted TLS server with correct hostname",
			serverTLS: tlscfg.Options{
				Enabled:  true,
				CertPath: "testdata/example-server-cert.pem",
				KeyPath:  "testdata/example-server-key.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/example-CA-cert.pem",
				ServerName: "example.com",
			},
			expectError:      false,
			expectServerFail: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverOptions := &QueryOptions{TLSHTTP: test.serverTLS}
			httpServer, err := createHTTPServer(&querysvc.QueryService{}, serverOptions, opentracing.NoopTracer{}, zap.NewNop())

			if test.expectServerFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, httpServer)
			}
			httpListener, err := net.Listen("tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP))
			if test.expectServerFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if serverOptions.TLSHTTP.Enabled {

				tlsCfg, _ := serverOptions.TLSHTTP.Config()
				tlsCfg.Rand = rand.Reader
				tlsHTTPListener := tls.NewListener(httpListener, tlsCfg)
				go func() {
					// err
					_ = httpServer.Serve(tlsHTTPListener)
					// if err != nil {
					// 	//pass
					// }

					// if test.expectServerFail {

					// 	//assert.Equal(t, false, (err == nil) || (err == http.ErrServerClosed))

					// }

				}()
				// defer httpServer.Close()
				// defer tlsHTTPListener.Close()
				// defer httpListener.Close()
				time.Sleep(10 * time.Millisecond) // wait for server to start serving

				clientTLSCfg, err0 := test.clientTLS.Config()
				require.NoError(t, err0)
				// fmt.Println(tlsCfg.ClientCAs != nil, tlsCfg.ClientAuth == tls.RequireAndVerifyClientCert)
				conn, err1 := tls.Dial("tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP), clientTLSCfg)

				if test.expectError {
					require.Error(t, err1)
				} else {
					require.NoError(t, err1)
					//defer conn.Close()
				}
				if conn != nil {
					require.Nil(t, conn.Close())
				}
				if httpServer != nil {
					require.Nil(t, httpServer.Close())
				}

			} else {
				go func() {
					// err :
					_ = httpServer.Serve(httpListener)
					// if err != nil {
					// 	//pass
					// }

					// if test.expectServerFail {

					// 	//assert.Equal(t, false, (err == nil) || (err == http.ErrServerClosed))

					// }
					// time.Sleep(2 * time.Second)
				}()

				time.Sleep(10 * time.Millisecond) // wait for server to start serving
				//defer httpServer.Close()
				conn, err2 := net.Dial("tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP))
				if test.expectError {
					require.Error(t, err2)
				} else {
					require.NoError(t, err2)
					//defer conn.Close()
				}
				if conn != nil {
					require.Nil(t, conn.Close())
				}
				if httpServer != nil {
					require.Nil(t, httpServer.Close())
				}

			}
			time.Sleep(50 * time.Millisecond)
		})
	}
}

func TestTLSHTTPServerWithMTLS(t *testing.T) {
	tests := []struct {
		name              string
		serverTLS         tlscfg.Options
		clientTLS         tlscfg.Options
		expectError       bool
		expectServerFail  bool
		expectClientError bool
	}{
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     "testdata/example-server-cert.pem",
				KeyPath:      "testdata/example-server-key.pem",
				ClientCAPath: "testdata/example-CA-cert.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/example-CA-cert.pem",
				ServerName: "example.com",
			},
			expectError:       false,
			expectServerFail:  false,
			expectClientError: true,
		},
		{
			name: "should pass with TLS client with cert to trusted TLS server requiring cert",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     "testdata/example-server-cert.pem",
				KeyPath:      "testdata/example-server-key.pem",
				ClientCAPath: "testdata/example-CA-cert.pem",
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   "testdata/example-client-cert.pem",
				KeyPath:    "testdata/example-client-key.pem",
			},
			expectError:       false,
			expectServerFail:  false,
			expectClientError: false,
		},
		{
			name: "should fail with TLS client without cert to trusted TLS server requiring cert from a different CA",
			serverTLS: tlscfg.Options{
				Enabled:      true,
				CertPath:     "testdata/example-server-cert.pem",
				KeyPath:      "testdata/example-server-key.pem",
				ClientCAPath: "testdata/wrong-CA-cert.pem", // NB: wrong CA
			},
			clientTLS: tlscfg.Options{
				Enabled:    true,
				CAPath:     "testdata/example-CA-cert.pem",
				ServerName: "example.com",
				CertPath:   "testdata/example-client-cert.pem",
				KeyPath:    "testdata/example-client-key.pem",
			},
			expectError:       false,
			expectServerFail:  false,
			expectClientError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverOptions := &QueryOptions{TLSHTTP: test.serverTLS}
			readMock := &spanstoremocks.Reader{}
			serverQuerySvc := querysvc.NewQueryService(readMock, &depsmocks.Reader{}, querysvc.QueryServiceOptions{})

			httpServer, err := createHTTPServer(serverQuerySvc, serverOptions, opentracing.NoopTracer{}, zap.NewNop())

			if test.expectServerFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, httpServer)
			}
			httpListener, err := net.Listen("tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP))
			if test.expectServerFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			tlsCfg, _ := serverOptions.TLSHTTP.Config()
			tlsCfg.Rand = rand.Reader
			tlsHTTPListener := tls.NewListener(httpListener, tlsCfg)
			go func() {
				// err :
				_ = httpServer.Serve(tlsHTTPListener)
				// if err != nil {
				// 	//pass
				// }

				// if test.expectServerFail {

				// 	assert.Equal(t, false, (err == nil) || (err == http.ErrServerClosed))

				// }

			}()
			// defer httpServer.Close()
			time.Sleep(10 * time.Millisecond) // wait for server to start serving

			clientTLSCfg, err0 := test.clientTLS.Config()
			require.NoError(t, err0)
			// fmt.Println(tlsCfg.ClientCAs != nil, tlsCfg.ClientAuth == tls.RequireAndVerifyClientCert)
			conn, err1 := tls.Dial("tcp", "localhost:"+fmt.Sprintf("%d", ports.QueryHTTP), clientTLSCfg)

			if test.expectError {
				require.Error(t, err1)

			} else {
				require.NoError(t, err1)
				conn.Close()
			}
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: clientTLSCfg,
				},
			}
			readMock.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return([]*model.Trace{mockTrace}, nil).Once()
			queryString := "/api/traces?service=service&start=0&end=0&operation=operation&limit=200&minDuration=20ms"
			req, err := http.NewRequest("GET", "https://localhost:"+fmt.Sprintf("%d", ports.QueryHTTP)+queryString, nil)
			assert.Nil(t, err)
			req.Header.Add("Accept", "application/json")

			resp, err2 := client.Do(req)
			if err2 == nil {
				resp.Body.Close()
			}
			httpServer.Close()

			if test.expectClientError {
				require.Error(t, err2)
			} else {
				require.NoError(t, err2)

			}

		})
	}
}
func TestServer(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	flagsSvc.Logger = zap.NewNop()
	hostPort := ports.GetAddressFromCLIOptions(ports.QueryHTTP, "")

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}
	expectedServices := []string{"test"}
	spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil)

	querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})

	server, err := NewServer(flagsSvc.Logger, querySvc,
		&QueryOptions{HostPort: hostPort, BearerTokenPropagation: true},
		opentracing.NoopTracer{})
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.SetHealthCheckStatus(s)
		}
	}()

	client := newGRPCClient(t, hostPort)
	defer client.conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.GetServices(ctx, &api_v2.GetServicesRequest{})
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, res.Services)

	server.Close()
	for i := 0; i < 10; i++ {
		if flagsSvc.HC().Get() == healthcheck.Unavailable {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal(t, healthcheck.Unavailable, flagsSvc.HC().Get())
}

func TestServerGracefulExit(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)

	zapCore, logs := observer.New(zap.ErrorLevel)
	assert.Equal(t, 0, logs.Len(), "Expected initial ObservedLogs to have zero length.")

	flagsSvc.Logger = zap.New(zapCore)

	querySvc := &querysvc.QueryService{}
	tracer := opentracing.NoopTracer{}
	server, err := NewServer(flagsSvc.Logger, querySvc, &QueryOptions{HostPort: ports.PortToHostPort(ports.QueryAdminHTTP)}, tracer)
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	go func() {
		for s := range server.HealthCheckStatus() {
			flagsSvc.SetHealthCheckStatus(s)
		}
	}()

	// Wait for servers to come up before we can call .Close()
	// TODO Find a way to wait only as long as necessary. Unconditional sleep slows down the tests.
	time.Sleep(1 * time.Second)
	server.Close()

	for _, logEntry := range logs.All() {
		assert.True(t, logEntry.Level != zap.ErrorLevel,
			"Error log found on server exit: %v", logEntry)
	}
}

func TestServerHandlesPortZero(t *testing.T) {
	flagsSvc := flags.NewService(ports.QueryAdminHTTP)
	zapCore, logs := observer.New(zap.InfoLevel)
	flagsSvc.Logger = zap.New(zapCore)

	querySvc := &querysvc.QueryService{}
	tracer := opentracing.NoopTracer{}
	server, err := NewServer(flagsSvc.Logger, querySvc, &QueryOptions{HostPort: ":0"}, tracer)
	assert.Nil(t, err)
	assert.NoError(t, server.Start())
	server.Close()

	message := logs.FilterMessage("Query server started")
	assert.Equal(t, 1, message.Len(), "Expected query started log message.")

	onlyEntry := message.All()[0]
	port := onlyEntry.ContextMap()["port"]
	assert.Greater(t, port, int64(0))
}
