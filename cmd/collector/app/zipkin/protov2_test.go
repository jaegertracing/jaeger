package zipkin

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	zmodel "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestProtoSpanFixtures(t *testing.T) {
	var spans zmodel.ListOfSpans
	loadJSON(t, "fixtures/zipkin_proto_01.json", &spans)
	fmt.Println(spans)
	tSpans, err := protoSpansV2ToThrift(&spans)
	require.NoError(t, err)
	assert.Equal(t, len(tSpans), 1)
	var pid int64 = 1
	var ts int64 = 1
	var d int64 = 10
	fmt.Println(tSpans)
	localE := &zipkincore.Endpoint{ServiceName: "foo", Ipv4: 170594602}
	remoteE := &zipkincore.Endpoint{ServiceName: "bar", Ipv4: 170594603}
	var highID = int64(4793352529331701374)
	fmt.Println(highID)
	tSpan := &zipkincore.Span{ID: 2, TraceID: int64(4795885597963667071), TraceIDHigh: &highID, ParentID: &pid, Name: "foo", Debug: true, Duration: &d, Timestamp: &ts,
		Annotations: []*zipkincore.Annotation{
			{Value: "foo", Timestamp: 1, Host: localE},
			{Value: zipkincore.CLIENT_SEND, Timestamp: ts, Host: localE},
			{Value: zipkincore.CLIENT_RECV, Timestamp: ts + d, Host: localE}},
		BinaryAnnotations: []*zipkincore.BinaryAnnotation{
			{Key: "foo", Value: []byte("bar"), Host: localE, AnnotationType: zipkincore.AnnotationType_STRING},
			{Key: zipkincore.SERVER_ADDR, Host: remoteE, AnnotationType: zipkincore.AnnotationType_BOOL}}}
	assert.Equal(t, tSpan, tSpans[0])
}

func TestLCFromProtoSpanLocalEndpoint(t *testing.T) {
	var spans zmodel.ListOfSpans
	loadProto(t, "fixtures/zipkin_proto_02.json", &spans)
	tSpans, err := protoSpansV2ToThrift(&spans)
	fmt.Println(tSpans)
	require.NoError(t, err)
	assert.Equal(t, len(tSpans), 1)
	var ts int64 = 1
	var d int64 = 10
	tSpan := &zipkincore.Span{ID: 2, TraceID: 2, Name: "foo", Duration: &d, Timestamp: &ts,
		BinaryAnnotations: []*zipkincore.BinaryAnnotation{
			{Key: zipkincore.LOCAL_COMPONENT, Host: &zipkincore.Endpoint{ServiceName: "bar", Ipv4: 170594602, Port: 8080},
				AnnotationType: zipkincore.AnnotationType_STRING},
		}}
	assert.Equal(t, tSpan, tSpans[0])
}

func loadProto(t *testing.T, fname string, spans *zmodel.ListOfSpans) {
	b, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	err = json.Unmarshal(b, spans)
	require.NoError(t, err)
}

func TestIdErrs(t *testing.T) {
	validID := randBytesOfLen(8)
	validTraceID := randBytesOfLen(16)
	invalidTraceID := randBytesOfLen(32)
	invalidParentID := randBytesOfLen(32)

	tests := []struct {
		span   zmodel.Span
		errMsg string
	}{
		{
			span:   zmodel.Span{Id: randBytesOfLen(16)},
			errMsg: "Invalid length for Span ID",
		},
		{
			span:   zmodel.Span{Id: validID, TraceId: invalidTraceID},
			errMsg: "TraceID cannot be longer than 16 bytes",
		},
		{
			span:   zmodel.Span{Id: validID, TraceId: validTraceID, ParentId: invalidParentID},
			errMsg: "Invalid length for Parent ID",
		},
	}

	for _, test := range tests {
		_, err := protoSpanV2ToThrift(&test.span)
		require.Error(t, err)
		assert.Equal(t, err.Error(), test.errMsg)
	}
}

func TestEndpointValueErrs(t *testing.T) {
	validID := randBytesOfLen(8)
	validTraceID := randBytesOfLen(16)
	invalidLocalEp := zmodel.Endpoint{Ipv4: randBytesOfLen(8)}
	invalidRemoteEp := zmodel.Endpoint{Ipv6: randBytesOfLen(8)}

	tests := []struct {
		span   zmodel.Span
		errMsg string
	}{
		{
			span:   zmodel.Span{Id: validID, TraceId: validTraceID, LocalEndpoint: &invalidLocalEp},
			errMsg: "Invalid length for Endpoint Ipv4",
		},
		{
			span:   zmodel.Span{Id: validID, TraceId: validTraceID, RemoteEndpoint: &invalidRemoteEp},
			errMsg: "Invalid length for Endpoint Ipv6",
		},
	}

	for _, test := range tests {
		_, err := protoSpanV2ToThrift(&test.span)
		require.Error(t, err)
		assert.Equal(t, err.Error(), test.errMsg)
	}
}

func randBytesOfLen(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// 1. Test list of spans

// 2. Test single span convertor
// Various cases to err

// 3. Test protoEndpointV2ToThrift

// 4. Check if other functions needs testing
