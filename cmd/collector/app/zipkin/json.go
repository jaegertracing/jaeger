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
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"

	"github.com/uber/jaeger/model"
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
	Endpoint endpoint    `json:"endpoint"`
	Key      string      `json:"key"`
	Value    interface{} `json:"value"`
	Type     string      `json:"type"`
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
	errWrongIpv4 = errors.New("wrong ipv4")
)

// DeserializeJSON deserialize zipkin v1 json spans into zipkin thrift
func DeserializeJSON(body []byte) ([]*zipkincore.Span, error) {
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
	id, err := model.SpanIDFromString(cutLongID(s.ID))
	if err != nil {
		return nil, err
	}
	traceID, err := model.TraceIDFromString(s.TraceID)
	if err != nil {
		return nil, err
	}

	tSpan := &zipkincore.Span{
		ID:        int64(id),
		TraceID:   int64(traceID.Low),
		Name:      s.Name,
		Debug:     s.Debug,
		Timestamp: s.Timestamp,
		Duration:  s.Duration,
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

// id can be padded with zeros. We let it fail later in case it's longer than 32
func cutLongID(id string) string {
	l := len(id)
	if l > 16 && l <= 32 {
		return id[:16]
	}
	return id
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

	var val []byte
	var valType zipkincore.AnnotationType
	switch ba.Type {
	case "BOOL":
		if ba.Value.(bool) {
			val = []byte{1}
		} else {
			val = []byte{0}
		}
		valType = zipkincore.AnnotationType_BOOL
	case "I16":
		buff := new(bytes.Buffer)
		binary.Write(buff, binary.LittleEndian, int16(ba.Value.(float64)))
		val = buff.Bytes()
		valType = zipkincore.AnnotationType_I16
	case "I32":
		buff := new(bytes.Buffer)
		binary.Write(buff, binary.LittleEndian, int32(ba.Value.(float64)))
		val = buff.Bytes()
		valType = zipkincore.AnnotationType_I32
	case "I64":
		buff := new(bytes.Buffer)
		binary.Write(buff, binary.LittleEndian, int64(ba.Value.(float64)))
		val = buff.Bytes()
		valType = zipkincore.AnnotationType_I64
	case "DOUBLE":
		val = float64bytes(ba.Value.(float64))
		valType = zipkincore.AnnotationType_DOUBLE
	case "BYTES":
		val, err = base64.StdEncoding.DecodeString(ba.Value.(string))
		if err != nil {
			return nil, err
		}
		valType = zipkincore.AnnotationType_BYTES
	case "STRING":
		fallthrough
	default:
		str := fmt.Sprintf("%s", ba.Value)
		val = []byte(str)
		fmt.Println("default")
		fmt.Println(str)
		valType = zipkincore.AnnotationType_STRING
	}

	return &zipkincore.BinaryAnnotation{
		Key:            ba.Key,
		Value:          val,
		Host:           endpoint,
		AnnotationType: valType,
	}, nil
}

// taken from https://stackoverflow.com/a/22492518/4158442
func float64bytes(float float64) []byte {
	bits := math.Float64bits(float)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, bits)
	return bytes
}

func parseIpv4(str string) (int32, error) {
	if str == "" {
		return 0, nil
	}

	ip := net.ParseIP(str).To4()
	if ip == nil {
		return 0, errWrongIpv4
	}

	var ipv4 int32
	for _, segment := range ip {
		parsed32 := int32(segment)
		ipv4 = ipv4<<8 | (parsed32 & 0xff)
	}

	return ipv4, nil
}
