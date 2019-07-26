package zipkin

import (
	"time"

	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
	"github.com/openzipkin/zipkin-go/model"
)

// Converts Zipkin Protobuf spans to Thrift model
func protoSpansV2ToThrift(zss []*model.SpanModel) ([]*zipkincore.Span, error) {
	tSpans := make([]*zipkincore.Span, 0, len(zss))
	for _, span := range zss {
		tSpan, err := protoSpanV2ToThrift(span)
		if err != nil {
			return nil, err
		}
		tSpans = append(tSpans, tSpan)
	}
	return tSpans, nil
}

func protoSpanV2ToThrift(s *model.SpanModel) (*zipkincore.Span, error) {
	// TODO
	sc := s.SpanContext
	ts := nanoToMicroSecs(s.Timestamp.UnixNano())
	d := nanoToMicroSecs(s.Duration.Nanoseconds())
	tSpan := &zipkincore.Span{
		ID:        int64(sc.ID),
		TraceID:   int64(sc.TraceID.Low),
		Name:      s.Name,
		Debug:     s.Debug,
		Timestamp: &ts,
		Duration:  &d,
	}
	if sc.TraceID.High != 0 {
		help := int64(sc.TraceID.High)
		tSpan.TraceIDHigh = &help
	}

	if len(sc.ParentID.String()) > 0 {
		signed := int64(*sc.ParentID)
		tSpan.ParentID = &signed
	}

	var localE *zipkincore.Endpoint
	if s.LocalEndpoint != nil {
		var err error
		localE, err = protoEndpointV2ToThrift(s.LocalEndpoint)
		if err != nil {
			return nil, err
		}
	}

	for _, a := range s.Annotations {
		tA := protoAnnoV2ToThrift(&a, localE)
		tSpan.Annotations = append(tSpan.Annotations, tA)
	}

	tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, tagsToThrift(s.Tags, localE)...)
	tSpan.Annotations = append(tSpan.Annotations, kindToThrift(ts, d, string(s.Kind), localE)...)

	if s.RemoteEndpoint != nil {
		rAddrAnno, err := protoRemoteEndpToThrift(s.RemoteEndpoint, s.Kind)
		if err != nil {
			return nil, err
		}
		tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, rAddrAnno)
	}

	// add local component to represent service name
	// to_domain looks for a service name in all [bin]annotations
	if localE != nil && len(tSpan.BinaryAnnotations) == 0 && len(tSpan.Annotations) == 0 {
		tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, &zipkincore.BinaryAnnotation{
			Key:            zipkincore.LOCAL_COMPONENT,
			Host:           localE,
			AnnotationType: zipkincore.AnnotationType_STRING,
		})
	}
	return tSpan, nil
}

func protoEndpointV2ToThrift(e *model.Endpoint) (*zipkincore.Endpoint, error) {
	if e == nil {
		return nil, nil
	}
	return eToThrift(string(e.IPv4), string(e.IPv6), int32(e.Port), e.ServiceName)
}

func protoAnnoV2ToThrift(a *model.Annotation, e *zipkincore.Endpoint) *zipkincore.Annotation {
	ts := nanoToMicroSecs(a.Timestamp.UnixNano())
	return &zipkincore.Annotation{
		Value:     a.Value,
		Timestamp: ts,
		Host:      e,
	}
}

func protoRemoteEndpToThrift(e *model.Endpoint, kind model.Kind) (*zipkincore.BinaryAnnotation, error) {
	rEndp, err := protoEndpointV2ToThrift(e)
	if err != nil {
		return nil, err
	}
	var key string
	switch kind {
	case model.Server:
		key = zipkincore.SERVER_ADDR
	case model.Client:
		key = zipkincore.CLIENT_ADDR
	case model.Consumer, model.Producer:
		key = zipkincore.MESSAGE_ADDR
	}

	return &zipkincore.BinaryAnnotation{
		Key:            key,
		Host:           rEndp,
		AnnotationType: zipkincore.AnnotationType_BOOL,
	}, nil
}

func nanoToMicroSecs(ns int64) int64 {
	return ns / int64(time.Millisecond)
}
