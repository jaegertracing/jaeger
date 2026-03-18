package jaegerai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// WsReadWriteCloser wraps a gorilla websocket to implement io.ReadWriteCloser
type WsReadWriteCloser struct {
	conn *websocket.Conn
	r    io.Reader
}

func NewWsAdapter(conn *websocket.Conn) *WsReadWriteCloser {
	return &WsReadWriteCloser{conn: conn}
}

func (w *WsReadWriteCloser) Read(p []byte) (int, error) {
	if w.r == nil {
		messageType, r, err := w.conn.NextReader()
		if err != nil {
			return 0, err
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			return 0, fmt.Errorf("unexpected message type: %d", messageType)
		}
		w.r = r
	}

	n, err := w.r.Read(p)
	if err == io.EOF {
		w.r = nil
		if n > 0 {
			return n, nil
		}
		return w.Read(p)
	}
	return n, err
}

func (w *WsReadWriteCloser) Write(p []byte) (int, error) {
	err := w.conn.WriteMessage(websocket.TextMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *WsReadWriteCloser) Close() error {
	return w.conn.Close()
}

// ChatRequest is the incoming payload
type ChatRequest struct {
	Prompt string `json:"prompt"`
}

// ChatHandler manages the AI gateway requests
type ChatHandler struct {
	Logger       *zap.Logger
	QueryService *querysvc.QueryService
}

func NewChatHandler(logger *zap.Logger, queryService *querysvc.QueryService) *ChatHandler {
	return &ChatHandler{Logger: logger, QueryService: queryService}
}

// streamingClient implements acp.Client to handle callbacks and streaming text
type streamingClient struct {
	w            http.ResponseWriter
	flusher      http.Flusher
	queryService *querysvc.QueryService
}

func searchTracesToolResult(ctx context.Context, queryService *querysvc.QueryService, query string) string {
	if queryService == nil {
		return `{"tool":"search_traces","error":"query service is not configured"}`
	}

	params := querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			// For this PoC we route tool calls through FindTraces with a deterministic service.
			ServiceName:   "dummy-service",
			OperationName: query,
		},
		RawTraces: true,
	}

	traces := make([]map[string]any, 0, 8)
	var iterErr error

	queryService.FindTraces(ctx, params)(func(batches []ptrace.Traces, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}

		for _, batch := range batches {
			rs := batch.ResourceSpans()
			for i := 0; i < rs.Len(); i++ {
				resourceSpans := rs.At(i)
				serviceName, _ := resourceSpans.Resource().Attributes().Get("service.name")
				ss := resourceSpans.ScopeSpans()
				for j := 0; j < ss.Len(); j++ {
					spans := ss.At(j).Spans()
					for k := 0; k < spans.Len(); k++ {
						span := spans.At(k)
						durationMs := (int64(span.EndTimestamp()) - int64(span.StartTimestamp())) / int64(time.Millisecond)
						traces = append(traces, map[string]any{
							"trace_id":    span.TraceID().String(),
							"span_id":     span.SpanID().String(),
							"service":     serviceName.Str(),
							"operation":   span.Name(),
							"duration_ms": durationMs,
						})
						if len(traces) >= 20 {
							return false
						}
					}
				}
			}
		}

		return len(traces) < 20
	})

	if iterErr != nil {
		return fmt.Sprintf(`{"tool":"search_traces","query":%q,"error":%q}`, query, iterErr.Error())
	}

	payload := map[string]any{
		"tool":   "search_traces",
		"query":  query,
		"traces": traces,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return `{"tool":"search_traces","error":"failed to encode result"}`
	}
	return string(b)
}

func parseSearchTracesQuery(path string) string {
	u, err := url.Parse(path)
	if err != nil {
		return ""
	}
	return u.Query().Get("q")
}

func (c *streamingClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	if len(p.Options) == 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Cancelled: &acp.RequestPermissionOutcomeCancelled{},
			},
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{OptionId: p.Options[0].OptionId},
		},
	}, nil
}

func (c *streamingClient) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	u := n.Update
	if u.AgentMessageChunk != nil {
		content := u.AgentMessageChunk.Content
		if content.Text != nil {
			c.w.Write([]byte(content.Text.Text))
			c.flusher.Flush()
		}
	}
	if u.ToolCall != nil {
		c.w.Write([]byte(fmt.Sprintf("\n[tool_call] %s\n", u.ToolCall.Title)))
		c.flusher.Flush()
	}
	if u.ToolCallUpdate != nil {
		c.w.Write([]byte(fmt.Sprintf("\n[tool_result] id=%s status=%s\n", u.ToolCallUpdate.ToolCallId, valueOrUnknown(u.ToolCallUpdate.Status))))
		c.flusher.Flush()
	}
	return nil
}

func valueOrUnknown(v *acp.ToolCallStatus) string {
	if v == nil {
		return "unknown"
	}
	return string(*v)
}

func (c *streamingClient) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, nil
}

func (c *streamingClient) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if strings.HasPrefix(p.Path, "acp://tool/search_traces") {
		query := parseSearchTracesQuery(p.Path)
		return acp.ReadTextFileResponse{Content: searchTracesToolResult(ctx, c.queryService, query)}, nil
	}

	return acp.ReadTextFileResponse{Content: fmt.Sprintf("unsupported path: %s", p.Path)}, nil
}

func (c *streamingClient) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{TerminalId: "t-1"}, nil
}

func (c *streamingClient) KillTerminalCommand(ctx context.Context, p acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	return acp.KillTerminalCommandResponse{}, nil
}

func (c *streamingClient) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *streamingClient) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{Output: "ok", Truncated: false}, nil
}

func (c *streamingClient) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, "ws://localhost:9000", nil)
	if err != nil {
		h.Logger.Error("Failed to dial ACP sidecar", zap.Error(err))
		http.Error(w, "Failed to connect to agent backend", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	adapter := NewWsAdapter(conn)

	clientImpl := &streamingClient{
		w:            w,
		flusher:      flusher,
		queryService: h.QueryService,
	}
	acpConn := acp.NewClientSideConnection(clientImpl, adapter, adapter)

	acpCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	_, err = acpConn.Initialize(acpCtx, acp.InitializeRequest{
		Meta: map[string]any{
			"tools": []map[string]string{
				{
					"name":        "search_traces",
					"description": "Returns traces from Jaeger for a query string",
					"call":        "fs/read_text_file path=acp://tool/search_traces?q=<query>",
				},
			},
		},
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapability{ReadTextFile: true, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{
			Name:    "jaeger-ai-gateway",
			Version: "0.1.0",
		},
	})
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Error initializing agent: %v\n", err)))
		return
	}

	sess, err := acpConn.NewSession(acpCtx, acp.NewSessionRequest{
		Cwd:        "/",
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Error creating session: %v\n", err)))
		return
	}

	// This is blocking until the agent finishes processing the prompt
	_, err = acpConn.Prompt(acpCtx, acp.PromptRequest{
		SessionId: sess.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(req.Prompt)},
	})
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Error starting prompt: %v\n", err)))
		return
	}
}
