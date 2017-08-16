// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
	Port        int16  `json:"port"`
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
type zipkinSpans []zipkinSpan

var (
	errIsNotUnsignedLog = errors.New("id is not an unsigned long")
)

func deserializeJSON(body []byte) ([]*zipkincore.Span, error) {
	spans, err := decode(body)
	if err != nil {
		return nil, err
	}

	return spansToThrift(spans)
}

func decode(body []byte) ([]zipkinSpan, error) {
	var spans zipkinSpans
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
	id, err := hexToUnsignedLong(s.ID)
	if err != nil {
		return nil, err
	}

	var traceID uint64
	if len(s.TraceID) == 32 {
		traceID, err = hexToUnsignedLong(s.TraceID[16:])
	} else {
		traceID, err = hexToUnsignedLong(s.TraceID)
	}
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

	return &zipkincore.Endpoint{
		ServiceName: e.ServiceName,
		Port:        e.Port,
		// TODO update zipkin.thrift to include ipv6
		Ipv4: ipv4,
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
	segments := strings.Split(str, ".")
	if len(segments) == 1 {
		return 0, nil
	}

	var ipv4 int32
	for _, segment := range segments {
		parsed, err := strconv.ParseInt(segment, 10, 32)
		if err != nil {
			return 0, err
		}

		parsed32 := int32(parsed)
		ipv4 = ipv4<<8 | (parsed32 & 0xff)
	}

	return ipv4, nil
}

func hexToUnsignedLong(hex string) (uint64, error) {
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
