package zipkin

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	zmodel "github.com/jaegertracing/jaeger/proto-gen/zipkin"
)

func mockListOfSpans() zmodel.ListOfSpans {
	id := make([]byte, 8)
	binary.BigEndian.PutUint64(id, uint64(2))
	tid := make([]byte, 8)
	binary.BigEndian.PutUint64(tid, uint64(4793352529331701374))
	pid := make([]byte, 8)
	binary.BigEndian.PutUint64(pid, uint64(1))
	ipv4 := make([]byte, 4)
	binary.BigEndian.PutUint32(ipv4, uint32(170594602))
	var ts uint64 = 1
	var d uint64 = 10
	span := &zmodel.Span{
		Id:        id,
		TraceId:   tid,
		ParentId:  pid,
		Name:      "foo",
		Debug:     true,
		Duration:  d,
		Timestamp: ts,
		LocalEndpoint: &zmodel.Endpoint{
			ServiceName: "bar",
			Ipv4:        ipv4,
		},
	}
	return zmodel.ListOfSpans{
		Spans: []*zmodel.Span{span},
	}
}

func TestProtoFixtures(t *testing.T) {
	var spans zmodel.ListOfSpans = mockListOfSpans()
	tSpans, err := protoSpansV2ToThrift(&spans)
	require.NoError(t, err)
	assert.Equal(t, len(tSpans), 1)
}
