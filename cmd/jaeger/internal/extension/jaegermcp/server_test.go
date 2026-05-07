// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "iter"
    "net/http"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    "go.opentelemetry.io/collector/component"
    "go.opentelemetry.io/collector/component/componenttest"
    "go.opentelemetry.io/collector/config/confighttp"
    "go.opentelemetry.io/collector/config/confignet"
    "go.opentelemetry.io/collector/extension"
    "go.opentelemetry.io/collector/pdata/pcommon"
    "go.opentelemetry.io/collector/pdata/ptrace"
    "go.uber.org/zap"

    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
    "github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
    "github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
    depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
    "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
    tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
    "github.com/jaegertracing/jaeger/internal/tenancy"
)

type mockQueryExtension struct {
    extension.Extension
    svc *querysvc.QueryService
    tm  *tenancy.Manager
}

func newMockQueryExtension(svc *querysvc.QueryService) *mockQueryExtension {
    if svc == nil {
        svc = querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
    }
    return &mockQueryExtension{
        svc: svc,
        tm:  tenancy.NewManager(&tenancy.Options{}),
    }
}

func (m *mockQueryExtension) QueryService() *querysvc.QueryService { return m.svc }
func (m *mockQueryExtension) TenancyManager() *tenancy.Manager     { return m.tm }
func (m *mockQueryExtension) MetricsReader() metricstore.Reader {
    r, _ := disabled.NewMetricsReader()
    return r
}

type mockHost struct {
    component.Host
    queryExt jaegerquery.Extension
}

func newMockHost() *mockHost {
    return &mockHost{Host: componenttest.NewNopHost(), queryExt: newMockQueryExtension(nil)}
}

func newMockHostWithQueryService(svc *querysvc.QueryService) *mockHost {
    return &mockHost{Host: componenttest.NewNopHost(), queryExt: newMockQueryExtension(svc)}
}

func newMockHostWithQueryServiceAndTenancy(svc *querysvc.QueryService, tm *tenancy.Manager) *mockHost {
    return &mockHost{Host: componenttest.NewNopHost(), queryExt: &mockQueryExtension{svc: svc, tm: tm}}
}

func (m *mockHost) GetExtensions() map[component.ID]component.Component {
    return map[component.ID]component.Component{jaegerquery.ID: m.queryExt}
}

func waitForServer(t *testing.T, addr string) {
    t.Helper()
    require.Eventually(t, func() bool {
        resp, err := http.Get(fmt.Sprintf("http://%s/mcp", addr))
        if err != nil {
            return false
        }
        require.NoError(t, resp.Body.Close())
        return true
    }, 1*time.Second, 10*time.Millisecond, "Server should be ready")
}

func startTestServerWithQueryService(t *testing.T, svc *querysvc.QueryService, logger *zap.Logger) (*server, string) {
    t.Helper()
    telset := componenttest.NewNopTelemetrySettings()
    if logger != nil {
        telset.Logger = logger
    }
    return startTestServerWithTelemetry(t, svc, telset)
}

func startTestServerWithTelemetry(t *testing.T, svc *querysvc.QueryService, telset component.TelemetrySettings) (*server, string) {
    t.Helper()
    host := newMockHostWithQueryService(svc)
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP},
        },
        ServerName:               "jaeger",
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    t.Cleanup(func() { assert.NoError(t, s.Shutdown(context.Background())) })
    addr := s.listener.Addr().String()
    waitForServer(t, addr)
    return s, addr
}

func startTestServer(t *testing.T) (*server, string) {
    t.Helper()
    return startTestServerWithQueryService(t, nil, nil)
}

func TestServerLifecycle(t *testing.T) {
    host := newMockHost()
    tests := []struct {
        name          string
        config        *Config
        expectedError string
    }{
        {
            name: "successful start and shutdown",
            config: &Config{
                HTTP:                     createDefaultConfig().(*Config).HTTP,
                ServerName:               "jaeger",
                ServerVersion:            "1.0.0",
                MaxSpanDetailsPerRequest: 20,
                MaxSearchResults:         100,
            },
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            telset := componenttest.NewNopTelemetrySettings()
            s := newServer(tt.config, telset)
            require.NotNil(t, s)
            err := s.Start(context.Background(), host)
            if tt.expectedError != "" {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.expectedError)
                return
            }
            require.NoError(t, err)
            assert.NoError(t, s.Shutdown(context.Background()))
        })
    }
}

func TestServerQueryServiceRetrieval(t *testing.T) {
    host := newMockHost()
    config := &Config{
        HTTP:                     createDefaultConfig().(*Config).HTTP,
        ServerName:               "jaeger",
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    telset := componenttest.NewNopTelemetrySettings()
    s := newServer(config, telset)
    require.NotNil(t, s)
    require.NoError(t, s.Start(context.Background(), host))
    require.NotNil(t, s.queryAPI, "queryAPI should be set after Start")
    assert.NoError(t, s.Shutdown(context.Background()))
}

func TestServerStartContinuesWhenMetricsMiddlewareFails(t *testing.T) {
    host := newMockHost()
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP},
        },
        ServerName:               "jaeger",
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    telset := componenttest.NewNopTelemetrySettings()
    telset.MeterProvider = &failingMeterProvider{failCounter: true}
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    t.Cleanup(func() { assert.NoError(t, s.Shutdown(context.Background())) })
}

func TestServerStartFailsWithoutQueryExtension(t *testing.T) {
    host := componenttest.NewNopHost()
    config := &Config{
        HTTP:                     createDefaultConfig().(*Config).HTTP,
        ServerName:               "jaeger",
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    telset := componenttest.NewNopTelemetrySettings()
    s := newServer(config, telset)
    err := s.Start(context.Background(), host)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "cannot find extension")
}

func TestServerStartFailsWithInvalidEndpoint(t *testing.T) {
    host := newMockHost()
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "invalid-endpoint-format", Transport: confignet.TransportTypeTCP},
        },
        ServerName:               "jaeger",
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    err := s.Start(context.Background(), host)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "failed to listen")
}

func TestServerMCPEndpoint(t *testing.T) {
    _, addr := startTestServer(t)
    resp, err := http.Get(fmt.Sprintf("http://%s/mcp", addr))
    require.NoError(t, err)
    defer resp.Body.Close()
    assert.NotEqual(t, http.StatusNotFound, resp.StatusCode)
    body, err := io.ReadAll(resp.Body)
    require.NoError(t, err)
    if resp.Header.Get("Content-Type") == "application/json" {
        var result map[string]any
        assert.NoError(t, json.Unmarshal(body, &result), "Response should be valid JSON")
    }
}

func TestServerShutdownWithError(t *testing.T) {
    host := newMockHost()
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP},
        },
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    s.listener.Close()
    time.Sleep(50 * time.Millisecond)
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
    defer cancel()
    <-ctx.Done()
    _ = s.Shutdown(ctx)
}

func TestServerShutdownAfterListenerClose(t *testing.T) {
    host := newMockHost()
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP},
        },
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    s.listener.Close()
    time.Sleep(50 * time.Millisecond)
    assert.NoError(t, s.Shutdown(context.Background()))
}

func TestServerShutdownErrorPath(t *testing.T) {
    host := newMockHost()
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP},
        },
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _ = s.Shutdown(ctx)
}

func TestServerServeFails(t *testing.T) {
    host := newMockHost()
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        HTTP: confighttp.ServerConfig{
            NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP},
        },
        ServerName:               "jaeger",
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    s.listener.Close()
    time.Sleep(100 * time.Millisecond)
    assert.NoError(t, s.Shutdown(context.Background()))
}

func TestServerDependencies(t *testing.T) {
    s := &server{}
    deps := s.Dependencies()
    require.Len(t, deps, 1)
    assert.Equal(t, jaegerquery.ID, deps[0])
}

func TestShutdownWithoutStart(t *testing.T) {
    telset := componenttest.NewNopTelemetrySettings()
    s := newServer(createDefaultConfig().(*Config), telset)
    assert.NoError(t, s.Shutdown(context.Background()))
}

func TestNewServer(t *testing.T) {
    telset := componenttest.NewNopTelemetrySettings()
    config := createDefaultConfig().(*Config)
    s := newServer(config, telset)
    assert.NotNil(t, s)
    assert.Equal(t, config, s.config)
    assert.Equal(t, telset, s.telset)
    assert.Nil(t, s.httpServer)
    assert.Nil(t, s.listener)
}

func TestHealthTool(t *testing.T) {
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        ServerName:               "test-server",
        ServerVersion:            "2.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    result, output, err := s.healthTool(context.Background(), nil, struct{}{})
    require.NoError(t, err)
    assert.Nil(t, result)
    assert.Equal(t, "ok", output.Status)
    assert.Equal(t, "test-server", output.Server)
    assert.Equal(t, "2.0.0", output.Version)
}

func TestSearchTracesToolIntegration(t *testing.T) {
    mockReader := &tracestoremocks.Reader{}
    mockReader.On("FindTraces", mock.Anything, mock.Anything).Return(
        func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
            return func(yield func([]ptrace.Traces, error) bool) {
                yield([]ptrace.Traces{createTestTraceForIntegration()}, nil)
            }
        },
    )
    qs := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
    _, addr := startTestServerWithQueryService(t, qs, nil)

    initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
    req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/mcp", addr), bytes.NewReader([]byte(initReq)))
    require.NoError(t, err)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json, text/event-stream")
    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()
    sessionID := resp.Header.Get("Mcp-Session-Id")
    require.NotEmpty(t, sessionID)
    body, err := io.ReadAll(resp.Body)
    require.NoError(t, err)
    t.Logf("Initialize response: %s", string(body))

    toolReq := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_traces","arguments":{"service_name":"test-service","start_time_min":"-1h"}}}`
    req2, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/mcp", addr), bytes.NewReader([]byte(toolReq)))
    require.NoError(t, err)
    req2.Header.Set("Content-Type", "application/json")
    req2.Header.Set("Accept", "application/json, text/event-stream")
    req2.Header.Set("Mcp-Session-Id", sessionID)
    resp2, err := http.DefaultClient.Do(req2)
    require.NoError(t, err)
    defer resp2.Body.Close()
    body2, err := io.ReadAll(resp2.Body)
    require.NoError(t, err)
    t.Logf("Tool call response: %s", string(body2))
    assert.Contains(t, string(body2), `"traces"`)
    assert.Contains(t, string(body2), "test-service")
    assert.NotContains(t, string(body2), `"traces":null`)

    delReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/mcp", addr), http.NoBody)
    require.NoError(t, err)
    delReq.Header.Set("Mcp-Session-Id", sessionID)
    http.DefaultClient.Do(delReq)
}

func TestSearchTracesToolEmptyResults(t *testing.T) {
    mockReader := &tracestoremocks.Reader{}
    mockReader.On("FindTraces", mock.Anything, mock.Anything).Return(
        func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
            return func(_ func([]ptrace.Traces, error) bool) {}
        },
    )
    qs := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
    _, addr := startTestServerWithQueryService(t, qs, nil)

    initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
    req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/mcp", addr), bytes.NewReader([]byte(initReq)))
    require.NoError(t, err)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json, text/event-stream")
    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()
    sessionID := resp.Header.Get("Mcp-Session-Id")
    require.NotEmpty(t, sessionID)
    io.ReadAll(resp.Body)

    toolReq := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_traces","arguments":{"service_name":"nonexistent","start_time_min":"-1h"}}}`
    req2, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/mcp", addr), bytes.NewReader([]byte(toolReq)))
    require.NoError(t, err)
    req2.Header.Set("Content-Type", "application/json")
    req2.Header.Set("Accept", "application/json, text/event-stream")
    req2.Header.Set("Mcp-Session-Id", sessionID)
    resp2, err := http.DefaultClient.Do(req2)
    require.NoError(t, err)
    defer resp2.Body.Close()
    body, err := io.ReadAll(resp2.Body)
    require.NoError(t, err)
    t.Logf("Empty result response: %s", string(body))
    assert.NotContains(t, string(body), `"traces":null`)
    assert.NotContains(t, string(body), `"traces": null`)

    delReq, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/mcp", addr), http.NoBody)
    require.NoError(t, err)
    delReq.Header.Set("Mcp-Session-Id", sessionID)
    http.DefaultClient.Do(delReq)
}

func createTestTraceForIntegration() ptrace.Traces {
    traces := ptrace.NewTraces()
    rs := traces.ResourceSpans().AppendEmpty()
    rs.Resource().Attributes().PutStr("service.name", "test-service")
    span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
    tid := pcommon.TraceID{}
    copy(tid[:], "12345678901234567890123456789012")
    span.SetTraceID(tid)
    span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
    span.SetParentSpanID(pcommon.SpanID{})
    span.SetName("/api/test")
    span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-5 * time.Second)))
    span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))
    span.Status().SetCode(ptrace.StatusCodeOk)
    return traces
}

func TestServerMCPEndpointEnforcesTenancy(t *testing.T) {
    tm := tenancy.NewManager(&tenancy.Options{Enabled: true, Header: "x-tenant", Tenants: []string{"tenant-a"}})
    host := newMockHostWithQueryServiceAndTenancy(nil, tm)
    telset := componenttest.NewNopTelemetrySettings()
    config := &Config{
        HTTP:                     confighttp.ServerConfig{NetAddr: confignet.AddrConfig{Endpoint: "localhost:0", Transport: confignet.TransportTypeTCP}},
        ServerVersion:            "1.0.0",
        MaxSpanDetailsPerRequest: 20,
        MaxSearchResults:         100,
    }
    s := newServer(config, telset)
    require.NoError(t, s.Start(context.Background(), host))
    t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
    addr := s.listener.Addr().String()
    resp, err := http.Get(fmt.Sprintf("http://%s/mcp", addr))
    require.NoError(t, err)
    defer resp.Body.Close()
    assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
    body, err := io.ReadAll(resp.Body)
    require.NoError(t, err)
    assert.Contains(t, string(body), "missing tenant header")
}




