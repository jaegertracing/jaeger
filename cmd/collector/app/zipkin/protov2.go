// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zipkin

import (
	"encoding/binary"
	"fmt"
	"net"

	model "github.com/jaegertracing/jaeger/model"
	zipkinProto "github.com/jaegertracing/jaeger/proto-gen/zipkin"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Converts Zipkin Protobuf spans to Thrift model
func protoSpansV2ToThrift(listOfSpans *zipkinProto.ListOfSpans) ([]*zipkincore.Span, error) {
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

func protoSpanV2ToThrift(s *zipkinProto.Span) (*zipkincore.Span, error) {
	if len(s.Id) != model.TraceIDShortBytesLen {
		return nil, fmt.Errorf("invalid length for Span ID")
	}
	id := binary.BigEndian.Uint64(s.Id)
	traceID := model.TraceID{}
	err := traceID.Unmarshal(s.TraceId)
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
		if len(s.ParentId) != model.TraceIDShortBytesLen {
			return nil, fmt.Errorf("invalid length for parentId")
		}
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
		rAddrAnno, err := protoRemoteEndpToAddrAnno(s.RemoteEndpoint, s.Kind)
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

func protoRemoteEndpToAddrAnno(e *zipkinProto.Endpoint, kind zipkinProto.Span_Kind) (*zipkincore.BinaryAnnotation, error) {
	rEndp, err := protoEndpointV2ToThrift(e)
	if err != nil {
		return nil, err
	}
	var key string
	switch kind {
	case zipkinProto.Span_CLIENT:
		key = zipkincore.SERVER_ADDR
	case zipkinProto.Span_SERVER:
		key = zipkincore.CLIENT_ADDR
	case zipkinProto.Span_CONSUMER, zipkinProto.Span_PRODUCER:
		key = zipkincore.MESSAGE_ADDR
	}

	return &zipkincore.BinaryAnnotation{
		Key:            key,
		Host:           rEndp,
		AnnotationType: zipkincore.AnnotationType_BOOL,
	}, nil
}

func protoKindToThrift(ts int64, d int64, kind zipkinProto.Span_Kind, localE *zipkincore.Endpoint) []*zipkincore.Annotation {
	var annos []*zipkincore.Annotation
	switch kind {
	case zipkinProto.Span_SERVER:
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
	case zipkinProto.Span_CLIENT:
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
	case zipkinProto.Span_PRODUCER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.MESSAGE_SEND,
			Host:      localE,
			Timestamp: ts,
		})
	case zipkinProto.Span_CONSUMER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.MESSAGE_RECV,
			Host:      localE,
			Timestamp: ts,
		})
	}
	return annos
}

func protoEndpointV2ToThrift(e *zipkinProto.Endpoint) (*zipkincore.Endpoint, error) {
	lv4 := len(e.Ipv4)
	if lv4 > 0 && lv4 != net.IPv4len {
		return nil, fmt.Errorf("wrong Ipv4")
	}
	lv6 := len(e.Ipv6)
	if lv6 > 0 && lv6 != net.IPv6len {
		return nil, fmt.Errorf("wrong Ipv6")
	}
	ipv4 := binary.BigEndian.Uint32(e.Ipv4)
	port := port(e.Port)
	return &zipkincore.Endpoint{
		ServiceName: e.ServiceName,
		Port:        int16(port),
		Ipv4:        int32(ipv4),
		Ipv6:        e.Ipv6,
	}, nil
}

func protoAnnoV2ToThrift(a *zipkinProto.Annotation, e *zipkincore.Endpoint) *zipkincore.Annotation {
	return &zipkincore.Annotation{
		Value:     a.Value,
		Timestamp: int64(a.Timestamp),
		Host:      e,
	}
}
