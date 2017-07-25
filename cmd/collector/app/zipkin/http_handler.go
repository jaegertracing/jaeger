package zipkin

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"io/ioutil"
	"strings"
	"strconv"
	"time"

	"go.uber.org/zap"
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"

	"github.com/uber/jaeger/cmd/collector/app"
	zipkincore "github.com/uber/jaeger/thrift-gen/zipkincore"
	tchanThrift "github.com/uber/tchannel-go/thrift"
	"github.com/uber/jaeger/cmd/collector/app/builder"
)

// APIHandler handles all HTTP calls to the collector
type APIHandler struct {
	zipkinSpansHandler   app.ZipkinSpansHandler
}

// NewAPIHandler returns a new APIHandler
func NewAPIHandler(
	zipkinSpansHandler app.ZipkinSpansHandler,
) *APIHandler {
	return &APIHandler{
		zipkinSpansHandler: zipkinSpansHandler,
	}
}

func (aH *APIHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/v1/spans", aH.zipkinHandler).Methods(http.MethodPost)
}

func (aH *APIHandler) zipkinHandler(w http.ResponseWriter, r *http.Request) {
	bRead := r.Body
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		result, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "Wrong encoding", http.StatusBadRequest)
		}
		bRead = result
	}

	bodyBytes, err := ioutil.ReadAll(bRead)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusBadRequest)
	}

	if r.Header.Get("Content-Type") == "application/x-thrift" {
		handleZipkinThrift(aH.zipkinSpansHandler, bodyBytes, w)
	} else {
		http.Error(w, "Only Content-Type:application/x-thrift is supported at the moment", http.StatusBadRequest)
	}

	w.WriteHeader(http.StatusAccepted)
}

func handleZipkinThrift(zHandler app.ZipkinSpansHandler, bodyBytes []byte, w http.ResponseWriter) {
	spans, err := deserializeZipkin(bodyBytes)
	if err != nil {
		http.Error(w, fmt.Sprintf(app.UnableToReadBodyErrFormat, err), http.StatusBadRequest)
		return
	}

	ctx, _ := tchanThrift.NewContext(time.Minute)
	if _, err = zHandler.SubmitZipkinBatch(ctx, spans); err != nil {
		http.Error(w, fmt.Sprintf("Cannot submit Zipkin batch: %v", err), http.StatusInternalServerError)
		return
	}
}

func deserializeZipkin(b []byte) ([]*zipkincore.Span, error) {
	buffer := thrift.NewTMemoryBuffer()
	buffer.Write(b)

	transport := thrift.NewTBinaryProtocolTransport(buffer)
	_, size, err := transport.ReadListBegin() // Ignore the returned element type
	if err != nil {
		return nil, err
	}

	// We don't depend on the size returned by ReadListBegin to preallocate the array because it
	// sometimes returns a nil error on bad input and provides an unreasonably large int for size
	var spans []*zipkincore.Span
	for i := 0; i < size; i++ {
		zs := &zipkincore.Span{}
		if err = zs.Read(transport); err != nil {
			return nil, err
		}
		spans = append(spans, zs)
	}

	return spans, nil
}

func StartHttpAPI(logger *zap.Logger, zipkinSpansHandler app.ZipkinSpansHandler, recoveryHandler func (http.Handler) http.Handler) {
	r := mux.NewRouter()
	NewAPIHandler(zipkinSpansHandler).RegisterRoutes(r)
	httpPortStr := ":" + strconv.Itoa(*builder.CollectorZipkinHTTPPort)
	logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", *builder.CollectorZipkinHTTPPort))
	if err := http.ListenAndServe(httpPortStr, recoveryHandler(r)); err != nil {
		logger.Fatal("Could not launch service", zap.Error(err))
	}
}

