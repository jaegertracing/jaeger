// Copyright (c) 2017 The Jaeger Authors.
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
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/swagger-gen/models"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func spansV2ToThrift(spans models.ListOfSpans) ([]*zipkincore.Span, error) {
	tSpans := make([]*zipkincore.Span, 0, len(spans))
	for _, span := range spans {
		tSpan, err := spanV2ToThrift(span)
		if err != nil {
			return nil, err
		}
		tSpans = append(tSpans, tSpan)
	}
	return tSpans, nil
}

func spanV2ToThrift(s *models.Span) (*zipkincore.Span, error) {
	id, err := model.SpanIDFromString(cutLongID(*s.ID))
	if err != nil {
		return nil, err
	}
	traceID, err := model.TraceIDFromString(*s.TraceID)
	if err != nil {
		return nil, err
	}
	tSpan := &zipkincore.Span{
		ID:        int64(id),
		TraceID:   int64(traceID.Low),
		Name:      s.Name,
		Debug:     s.Debug,
		Timestamp: &s.Timestamp,
		Duration:  &s.Duration,
	}
	if traceID.High != 0 {
		help := int64(traceID.High)
		tSpan.TraceIDHigh = &help
	}

	if len(s.ParentID) > 0 {
		parentID, err := model.SpanIDFromString(cutLongID(s.ParentID))
		if err != nil {
			return nil, err
		}
		signed := int64(parentID)
		tSpan.ParentID = &signed
	}

	var localE *zipkincore.Endpoint
	if s.LocalEndpoint != nil {
		localE, err = endpointV2ToThrift(s.LocalEndpoint)
		if err != nil {
			return nil, err
		}
	}

	for _, a := range s.Annotations {
		tA := annoV2ToThrift(a, localE)
		tSpan.Annotations = append(tSpan.Annotations, tA)
	}

	tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, tagsToThrift(s.Tags, localE)...)
	tSpan.Annotations = append(tSpan.Annotations, kindToThrift(s.Timestamp, s.Duration, s.Kind, localE)...)

	if s.RemoteEndpoint != nil {
		rAddrAnno, err := remoteEndpToThrift(s.RemoteEndpoint, s.Kind)
		if err != nil {
			return nil, err
		}
		if rAddrAnno != nil {
			tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, rAddrAnno)
		}
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

func remoteEndpToThrift(e *models.Endpoint, kind string) (*zipkincore.BinaryAnnotation, error) {
	rEndp, err := endpointV2ToThrift(e)
	if err != nil {
		return nil, err
	}
	var key string
	switch kind {
	case models.SpanKindCLIENT:
		key = zipkincore.SERVER_ADDR
	case models.SpanKindSERVER:
		key = zipkincore.CLIENT_ADDR
	case models.SpanKindCONSUMER, models.SpanKindPRODUCER:
		key = zipkincore.MESSAGE_ADDR
	default:
		return nil, nil
	}

	return &zipkincore.BinaryAnnotation{
		Key:            key,
		Host:           rEndp,
		AnnotationType: zipkincore.AnnotationType_BOOL,
	}, nil
}

func kindToThrift(ts int64, d int64, kind string, localE *zipkincore.Endpoint) []*zipkincore.Annotation {
	var annos []*zipkincore.Annotation
	switch kind {
	case models.SpanKindSERVER:
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
	case models.SpanKindCLIENT:
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
	case models.SpanKindPRODUCER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.MESSAGE_SEND,
			Host:      localE,
			Timestamp: ts,
		})
	case models.SpanKindCONSUMER:
		annos = append(annos, &zipkincore.Annotation{
			Value:     zipkincore.MESSAGE_RECV,
			Host:      localE,
			Timestamp: ts,
		})
	}
	return annos
}

func endpointV2ToThrift(e *models.Endpoint) (*zipkincore.Endpoint, error) {
	if e == nil {
		return nil, nil
	}
	return eToThrift(string(e.IPV4), string(e.IPV6), int32(e.Port), e.ServiceName)
}

func annoV2ToThrift(a *models.Annotation, e *zipkincore.Endpoint) *zipkincore.Annotation {
	return &zipkincore.Annotation{
		Value:     a.Value,
		Timestamp: a.Timestamp,
		Host:      e,
	}
}

func tagsToThrift(tags models.Tags, localE *zipkincore.Endpoint) []*zipkincore.BinaryAnnotation {
	bAnnos := make([]*zipkincore.BinaryAnnotation, 0, len(tags))
	for k, v := range tags {
		ba := &zipkincore.BinaryAnnotation{
			Key:            k,
			Value:          []byte(v),
			AnnotationType: zipkincore.AnnotationType_STRING,
			Host:           localE,
		}
		bAnnos = append(bAnnos, ba)
	}
	return bAnnos
}
