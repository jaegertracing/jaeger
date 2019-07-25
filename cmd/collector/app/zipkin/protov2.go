package zipkin

import "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
import "github.com/openzipkin/zipkin-go/model"

// Converts Zipkin Protobuf spans to Thrift model
func protoSpansV2ToThrift(zss []*model.SpanModel) ([]*zipkincore.Span, error) {
	// TODO

	return nil, nil
}
