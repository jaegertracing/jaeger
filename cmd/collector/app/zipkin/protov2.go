package zipkin

import (
	"encoding/binary"
	"fmt"

	"github.com/jaegertracing/jaeger/model"
	zmodel "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Converts Zipkin Protobuf spans to Thrift model
func protoSpansV2ToThrift(listOfSpans *zmodel.ListOfSpans) ([]*zipkincore.Span, error) {
	tSpans := make([]*zipkincore.Span, 0, len(listOfSpans.Spans))
	for _, span := range listOfSpans.Spans {
		tSpan, err := protoSpanV2ToThrift(span)
		if err != nil {
			return nil, err
		}
		tSpans = append(tSpans, tSpan)
	}
	return tSpans, nil
}

func protoSpanV2ToThrift(s *zmodel.Span) (*zipkincore.Span, error) {
	id := binary.BigEndian.Uint64(s.Id)
	traceID, err := model.TraceIDFromString(fmt.Sprintf("%x", s.TraceId))
	if err != nil {
		return nil, err
	}
	ts, d := int64(s.Timestamp), int64(s.Duration)
	tSpan := &zipkincore.Span{
		ID:        int64(id),
		TraceID:   int64(traceID.Low),
		Name:      s.Name,
		Debug:     s.Debug,
		Timestamp: &ts,
		Duration:  &d,
	}
	if traceID.High != 0 {
		help := int64(traceID.High)
		tSpan.TraceIDHigh = &help
	}

	if len(s.ParentId) > 0 {
		parentID := binary.BigEndian.Uint64(s.ParentId)
		signed := int64(parentID)
		tSpan.ParentID = &signed
	}

	var localE *zipkincore.Endpoint
	if s.LocalEndpoint != nil {
		localE, err = protoEndpointV2ToThrift(s.LocalEndpoint)
		if err != nil {
			return nil, err
		}
	}

	for _, a := range s.Annotations {
		tA := protoAnnoV2ToThrift(a, localE)
		tSpan.Annotations = append(tSpan.Annotations, tA)
	}

	tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, tagsToThrift(s.Tags, localE)...)
	tSpan.Annotations = append(tSpan.Annotations, protoKindToThrift(ts, d, s.Kind, localE)...)

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

func protoRemoteEndpToThrift(e *zmodel.Endpoint, kind zmodel.Span_Kind) (*zipkincore.BinaryAnnotation, error) {
	rEndp, err := protoEndpointV2ToThrift(e)
	if err != nil {
		return nil, err
	}
	var key string
	switch kind {
	case zmodel.Span_CLIENT:
		key = zipkincore.SERVER_ADDR
	case zmodel.Span_SERVER:
		key = zipkincore.CLIENT_ADDR
	case zmodel.Span_CONSUMER, zmodel.Span_PRODUCER:
		key = zipkincore.MESSAGE_ADDR
	}

	return &zipkincore.BinaryAnnotation{
		Key:            key,
		Host:           rEndp,
		AnnotationType: zipkincore.AnnotationType_BOOL,
	}, nil
}

func protoKindToThrift(ts int64, d int64, kind zmodel.Span_Kind, localE *zipkincore.Endpoint) []*zipkincore.Annotation {
	var annos []*zipkincore.Annotation
	switch kind {
	case zmodel.Span_SERVER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.SERVER_RECV,
			Host:      localE,
			Timestamp: ts,
		})
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.SERVER_SEND,
			Host:      localE,
			Timestamp: ts + d,
		})
	case zmodel.Span_CLIENT:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.CLIENT_SEND,
			Host:      localE,
			Timestamp: ts,
		})
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.CLIENT_RECV,
			Host:      localE,
			Timestamp: ts + d,
		})
	case zmodel.Span_PRODUCER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.MESSAGE_SEND,
			Host:      localE,
			Timestamp: ts,
		})
	case zmodel.Span_CONSUMER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.MESSAGE_RECV,
			Host:      localE,
			Timestamp: ts,
		})
	}
	return annos
}

func protoEndpointV2ToThrift(e *zmodel.Endpoint) (*zipkincore.Endpoint, error) {
	if e == nil {
		return nil, nil
	}
	return eToThrift(string(e.Ipv4), string(e.Ipv6), int32(e.Port), e.ServiceName)
}

func protoAnnoV2ToThrift(a *zmodel.Annotation, e *zipkincore.Endpoint) *zipkincore.Annotation {
	return &zipkincore.Annotation{
		Value:     a.Value,
		Timestamp: int64(a.Timestamp),
		Host:      e,
	}
}
