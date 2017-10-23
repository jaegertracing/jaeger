// Copyright (c) 2017 Uber Technologies, Inc.
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
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

type endpoint struct {
	ServiceName string `json:"serviceName"`
	IPv4        string `json:"ipv4"`
	IPv6        string `json:"ipv6"`
	Port        int32  `json:"port"`
}
type annotation struct {
	Endpoint  endpoint `json:"endpoint"`
	Value     string   `json:"value"`
	Timestamp int64    `json:"timestamp"`
}
type binaryAnnotation struct {
	Endpoint endpoint `json:"endpoint"`
	Key      string   `json:"key"`
	Value    string   `json:"value"`
}
type zipkinSpan struct {
	ID                string             `json:"id"`
	ParentID          string             `json:"parentId,omitempty"`
	TraceID           string             `json:"traceId"`
	Name              string             `json:"name"`
	Timestamp         *int64             `json:"timestamp"`
	Duration          *int64             `json:"duration"`
	Debug             bool               `json:"debug"`
	Annotations       []annotation       `json:"annotations"`
	BinaryAnnotations []binaryAnnotation `json:"binaryAnnotations"`
}

var (
	errIsNotUnsignedLog = errors.New("id is not an unsigned long")
	errWrongIpv4        = errors.New("wrong ipv4")
)

func deserializeJSON(body []byte) ([]*zipkincore.Span, error) {
	spans, err := decode(body)
	if err != nil {
		return nil, err
	}

	return spansToThrift(spans)
}

func decode(body []byte) ([]zipkinSpan, error) {
	var spans []zipkinSpan
	if err := json.Unmarshal(body, &spans); err != nil {
		return nil, err
	}
	return spans, nil
}

func spansToThrift(spans []zipkinSpan) ([]*zipkincore.Span, error) {
	var tSpans []*zipkincore.Span
	for _, span := range spans {
		tSpan, err := spanToThrift(span)
		if err != nil {
			return nil, err
		}
		tSpans = append(tSpans, tSpan)
	}
	return tSpans, nil
}

func spanToThrift(s zipkinSpan) (*zipkincore.Span, error) {
	// TODO use model.SpanIDFromString and model.TraceIDFromString
	id, err := hexToUnsignedLong(s.ID)
	if err != nil {
		return nil, err
	}
	traceID, err := hexToUnsignedLong(s.TraceID)
	if err != nil {
		return nil, err
	}

	tSpan := &zipkincore.Span{
		ID:        int64(id),
		TraceID:   int64(traceID),
		Name:      s.Name,
		Debug:     s.Debug,
		Timestamp: s.Timestamp,
		Duration:  s.Duration,
	}

	if len(s.TraceID) == 32 {
		// take 0-16
		traceIDHigh, err := hexToUnsignedLong(s.TraceID[:16])
		if err != nil {
			return nil, err
		}
		help := int64(traceIDHigh)
		tSpan.TraceIDHigh = &help
	}

	if len(s.ParentID) > 0 {
		parentID, err := hexToUnsignedLong(s.ParentID)
		if err != nil {
			return nil, err
		}
		signed := int64(parentID)
		tSpan.ParentID = &signed
	}

	for _, a := range s.Annotations {
		tA, err := annoToThrift(a)
		if err != nil {
			return nil, err
		}
		tSpan.Annotations = append(tSpan.Annotations, tA)
	}
	for _, ba := range s.BinaryAnnotations {
		tBa, err := binAnnoToThrift(ba)
		if err != nil {
			return nil, err
		}
		tSpan.BinaryAnnotations = append(tSpan.BinaryAnnotations, tBa)
	}

	return tSpan, nil
}

func endpointToThrift(e endpoint) (*zipkincore.Endpoint, error) {
	ipv4, err := parseIpv4(e.IPv4)
	if err != nil {
		return nil, err
	}
	port := e.Port
	if port >= (1 << 15) {
		// Zipkin.thrift defines port as i16, so values between (2^15 and 2^16-1) must be encoded as negative
		port = port - (1 << 16)
	}

	return &zipkincore.Endpoint{
		ServiceName: e.ServiceName,
		Port:        int16(port),
		Ipv4:        ipv4,
		Ipv6:        []byte(e.IPv6),
	}, nil
}

func annoToThrift(a annotation) (*zipkincore.Annotation, error) {
	endpoint, err := endpointToThrift(a.Endpoint)
	if err != nil {
		return nil, err
	}

	return &zipkincore.Annotation{
		Timestamp: a.Timestamp,
		Value:     a.Value,
		Host:      endpoint,
	}, nil
}

func binAnnoToThrift(ba binaryAnnotation) (*zipkincore.BinaryAnnotation, error) {
	endpoint, err := endpointToThrift(ba.Endpoint)
	if err != nil {
		return nil, err
	}

	return &zipkincore.BinaryAnnotation{
		Key:            ba.Key,
		Value:          []byte(ba.Value),
		Host:           endpoint,
		AnnotationType: zipkincore.AnnotationType_STRING,
	}, nil
}

func parseIpv4(str string) (int32, error) {
	// TODO use net.ParseIP
	segments := strings.Split(str, ".")
	if len(segments) == 1 {
		return 0, nil
	}

	var ipv4 int32
	for _, segment := range segments {
		parsed, err := strconv.ParseInt(segment, 10, 32)
		if err != nil {
			return 0, errWrongIpv4
		}
		parsed32 := int32(parsed)
		ipv4 = ipv4<<8 | (parsed32 & 0xff)
	}

	return ipv4, nil
}

func hexToUnsignedLong(hex string) (uint64, error) {
	// TODO remove this func in favor of model.XxxFromString methods
	len := len(hex)
	if len < 1 || len > 32 {
		return 0, errIsNotUnsignedLog
	}

	start := 0
	if len > 16 {
		start = len - 16
	}

	var result uint64
	for i := start; i < len; i++ {
		c := hex[i]
		result <<= 4
		if c >= '0' && c <= '9' {
			result = result | uint64(c-'0')
		} else if c >= 'a' && c <= 'f' {
			result = result | uint64(c-'a'+10)
		} else {
			return 0, errIsNotUnsignedLog
		}
	}

	return result, nil
}
