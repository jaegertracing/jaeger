package sampling

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/uber/jaeger-lib/metrics"

	tSampling "github.com/uber/jaeger/thrift-gen/sampling"
)

const mimeTypeApplicationJSON = "application/json"

// NewSamplingServer creates a new server that hosts an HTTP/JSON endpoint for clients
// to query for sampling strategies.
func NewSamplingServer(hostPort string, manager Manager, mFactory metrics.Factory) *http.Server {
	handler := newSamplingHandler(manager, mFactory)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handler.serveHTTP(w, r, true /* thriftEnums092 */)
	})
	mux.HandleFunc("/sampling", func(w http.ResponseWriter, r *http.Request) {
		handler.serveHTTP(w, r, false /* thriftEnums092 */)
	})
	return &http.Server{Addr: hostPort, Handler: mux}
}

func newSamplingHandler(manager Manager, mFactory metrics.Factory) *samplingHandler {
	handler := &samplingHandler{manager: manager}
	metrics.Init(&handler.metrics, mFactory, nil)
	return handler
}

type samplingHandler struct {
	manager Manager
	metrics struct {
		// Number of good sampling requests
		GoodRequest metrics.Counter `metric:"sampling-server.requests"`

		// Number of good sampling requests against the old endpoint / using Thrift 0.9.2 enum codes
		LegacyRequestThrift092 metrics.Counter `metric:"sampling-server.requests-thrift-092"`

		// Number of bad sampling requests
		BadRequest metrics.Counter `metric:"sampling-server.bad-requests"`

		// Number of bad server responses
		BadServerResponse metrics.Counter `metric:"sampling-server.bad-server-responses"`

		// Number of bad sampling requests due to malformed thrift
		BadThrift metrics.Counter `metric:"sampling-server.bad-thrift"`

		// Number of failed response writes from sampling server
		WriteError metrics.Counter `metric:"sampling-server.write-errors"`
	}
}

func (h *samplingHandler) serveHTTP(w http.ResponseWriter, r *http.Request, thriftEnums092 bool) {
	services := r.URL.Query()["service"]
	if len(services) == 0 {
		h.metrics.BadRequest.Inc(1)
		http.Error(w, "'service' parameter is empty", http.StatusBadRequest)
		return
	}
	if len(services) > 1 {
		h.metrics.BadRequest.Inc(1)
		http.Error(w, "'service' parameter must occur only once", http.StatusBadRequest)
		return
	}
	resp, err := h.manager.GetSamplingStrategy(services[0])
	if err != nil {
		h.metrics.BadServerResponse.Inc(1)
		http.Error(w, fmt.Sprintf("tcollector error: %+v", err), http.StatusInternalServerError)
		return
	}
	json, err := json.Marshal(resp)
	if err != nil {
		h.metrics.BadThrift.Inc(1)
		http.Error(w, "Cannot marshall Thrift to JSON", http.StatusInternalServerError)
		return
	}
	if thriftEnums092 {
		json = h.encodeThriftEnums092(json)
	}
	w.Header().Add("Content-Type", mimeTypeApplicationJSON)
	if _, err := w.Write(json); err != nil {
		h.metrics.WriteError.Inc(1)
		return
	}
	if thriftEnums092 {
		h.metrics.LegacyRequestThrift092.Inc(1)
	} else {
		h.metrics.GoodRequest.Inc(1)
	}
}

var samplingStrategyTypes = []tSampling.SamplingStrategyType{
	tSampling.SamplingStrategyType_PROBABILISTIC,
	tSampling.SamplingStrategyType_RATE_LIMITING,
}

// Replace string enum values produced from Thrift 0.9.3 generated classes
// with integer codes produced from Thrift 0.9.2 generated classes.
//
// For example:
//
// Thrift 0.9.2 classes generate this JSON:
// {"strategyType":0,"probabilisticSampling":{"samplingRate":0.5},"rateLimitingSampling":null,"operationSampling":null}
//
// Thrift 0.9.3 classes generate this JSON:
// {"strategyType":"PROBABILISTIC","probabilisticSampling":{"samplingRate":0.5}}
func (h *samplingHandler) encodeThriftEnums092(json []byte) []byte {
	str := string(json)
	for _, strategyType := range samplingStrategyTypes {
		str = strings.Replace(
			str,
			fmt.Sprintf(`"strategyType":"%s"`, strategyType.String()),
			fmt.Sprintf(`"strategyType":%d`, strategyType),
			1,
		)
	}
	return []byte(str)
}
