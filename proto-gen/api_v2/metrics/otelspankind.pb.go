// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: otelspankind.proto

package metrics

import (
	fmt "fmt"
	math "math"

	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

// SpanKind is the type of span. Can be used to specify additional relationships between spans
// in addition to a parent/child relationship.
type SpanKind int32

const (
	// Unspecified. Do NOT use as default.
	// Implementations MAY assume SpanKind to be INTERNAL when receiving UNSPECIFIED.
	SpanKind_SPAN_KIND_UNSPECIFIED SpanKind = 0
	// Indicates that the span represents an internal operation within an application,
	// as opposed to an operation happening at the boundaries. Default value.
	SpanKind_SPAN_KIND_INTERNAL SpanKind = 1
	// Indicates that the span covers server-side handling of an RPC or other
	// remote network request.
	SpanKind_SPAN_KIND_SERVER SpanKind = 2
	// Indicates that the span describes a request to some remote service.
	SpanKind_SPAN_KIND_CLIENT SpanKind = 3
	// Indicates that the span describes a producer sending a message to a broker.
	// Unlike CLIENT and SERVER, there is often no direct critical path latency relationship
	// between producer and consumer spans. A PRODUCER span ends when the message was accepted
	// by the broker while the logical processing of the message might span a much longer time.
	SpanKind_SPAN_KIND_PRODUCER SpanKind = 4
	// Indicates that the span describes consumer receiving a message from a broker.
	// Like the PRODUCER kind, there is often no direct critical path latency relationship
	// between producer and consumer spans.
	SpanKind_SPAN_KIND_CONSUMER SpanKind = 5
)

var SpanKind_name = map[int32]string{
	0: "SPAN_KIND_UNSPECIFIED",
	1: "SPAN_KIND_INTERNAL",
	2: "SPAN_KIND_SERVER",
	3: "SPAN_KIND_CLIENT",
	4: "SPAN_KIND_PRODUCER",
	5: "SPAN_KIND_CONSUMER",
}

var SpanKind_value = map[string]int32{
	"SPAN_KIND_UNSPECIFIED": 0,
	"SPAN_KIND_INTERNAL":    1,
	"SPAN_KIND_SERVER":      2,
	"SPAN_KIND_CLIENT":      3,
	"SPAN_KIND_PRODUCER":    4,
	"SPAN_KIND_CONSUMER":    5,
}

func (x SpanKind) String() string {
	return proto.EnumName(SpanKind_name, int32(x))
}

func (SpanKind) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_77f837d0289d1179, []int{0}
}

func init() {
	proto.RegisterEnum("jaeger.api_v2.metrics.SpanKind", SpanKind_name, SpanKind_value)
}

func init() { proto.RegisterFile("otelspankind.proto", fileDescriptor_77f837d0289d1179) }

var fileDescriptor_77f837d0289d1179 = []byte{
	// 224 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x12, 0xca, 0x2f, 0x49, 0xcd,
	0x29, 0x2e, 0x48, 0xcc, 0xcb, 0xce, 0xcc, 0x4b, 0xd1, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x12,
	0xcd, 0x4a, 0x4c, 0x4d, 0x4f, 0x2d, 0xd2, 0x4b, 0x2c, 0xc8, 0x8c, 0x2f, 0x33, 0xd2, 0xcb, 0x4d,
	0x2d, 0x29, 0xca, 0x4c, 0x2e, 0x96, 0x12, 0x49, 0xcf, 0x4f, 0xcf, 0x07, 0xab, 0xd0, 0x07, 0xb1,
	0x20, 0x8a, 0xb5, 0x66, 0x32, 0x72, 0x71, 0x04, 0x17, 0x24, 0xe6, 0x79, 0x67, 0xe6, 0xa5, 0x08,
	0x49, 0x72, 0x89, 0x06, 0x07, 0x38, 0xfa, 0xc5, 0x7b, 0x7b, 0xfa, 0xb9, 0xc4, 0x87, 0xfa, 0x05,
	0x07, 0xb8, 0x3a, 0x7b, 0xba, 0x79, 0xba, 0xba, 0x08, 0x30, 0x08, 0x89, 0x71, 0x09, 0x21, 0xa4,
	0x3c, 0xfd, 0x42, 0x5c, 0x83, 0xfc, 0x1c, 0x7d, 0x04, 0x18, 0x85, 0x44, 0xb8, 0x04, 0x10, 0xe2,
	0xc1, 0xae, 0x41, 0x61, 0xae, 0x41, 0x02, 0x4c, 0xa8, 0xa2, 0xce, 0x3e, 0x9e, 0xae, 0x7e, 0x21,
	0x02, 0xcc, 0xa8, 0x66, 0x04, 0x04, 0xf9, 0xbb, 0x84, 0x3a, 0xbb, 0x06, 0x09, 0xb0, 0xa0, 0x8a,
	0x3b, 0xfb, 0xfb, 0x05, 0x87, 0xfa, 0xba, 0x06, 0x09, 0xb0, 0x3a, 0x99, 0x9d, 0x78, 0x24, 0xc7,
	0x78, 0xe1, 0x91, 0x1c, 0xe3, 0x83, 0x47, 0x72, 0x8c, 0x5c, 0xf2, 0x99, 0xf9, 0x7a, 0x10, 0x9f,
	0x95, 0x14, 0x25, 0x26, 0x67, 0xe6, 0xa5, 0xa3, 0x79, 0x30, 0x8a, 0x1d, 0xca, 0x48, 0x62, 0x03,
	0x7b, 0xcd, 0x18, 0x10, 0x00, 0x00, 0xff, 0xff, 0x87, 0x34, 0x50, 0x06, 0x1d, 0x01, 0x00, 0x00,
}
