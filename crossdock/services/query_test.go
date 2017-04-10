package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	ui "github.com/uber/jaeger/model/json"
)

type testQueryHandler struct{}

func (h *testQueryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	svc := r.FormValue("service")
	body := []byte("bad json")
	if svc == "svc" {
		response := response{
			Data: []*ui.Trace{
				{TraceID: "traceid"},
			},
		}
		body, _ = json.Marshal(response)
	}
	w.Write(body)
}

func TestGetTraces(t *testing.T) {
	handler := &testQueryHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	query := &QueryService{url: server.URL, logger: zap.NewNop()}
	traces, err := query.GetTraces("svc", "op", map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.Len(t, traces, 1)
	assert.EqualValues(t, "traceid", traces[0].TraceID)

	traces, err = query.GetTraces("bad_svc", "op", map[string]string{"key": "value"})
	assert.Error(t, err)
}
