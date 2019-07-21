// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: model/proto/api_v2/collector.proto

/*
	Package api_v2 is a generated protocol buffer package.

	It is generated from these files:
		model/proto/api_v2/collector.proto
		model/proto/api_v2/query.proto
		model/proto/api_v2/sampling.proto
		model/proto/api_v2/throttling.proto

	It has these top-level messages:
		PostSpansRequest
		PostSpansResponse
		GetTraceRequest
		SpansResponseChunk
		ArchiveTraceRequest
		ArchiveTraceResponse
		TraceQueryParameters
		FindTracesRequest
		GetServicesRequest
		GetServicesResponse
		GetOperationsRequest
		GetOperationsResponse
		GetDependenciesRequest
		GetDependenciesResponse
		ProbabilisticSamplingStrategy
		RateLimitingSamplingStrategy
		OperationSamplingStrategy
		PerOperationSamplingStrategies
		SamplingStrategyResponse
		SamplingStrategyParameters
		ThrottlingConfig
		ServiceThrottlingConfig
		ThrottlingResponse
		ThrottlingConfigParameters
*/
package api_v2

import proto "github.com/gogo/protobuf/proto"
import golang_proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import jaeger_api_v2 "github.com/jaegertracing/jaeger/model"
import _ "github.com/gogo/protobuf/gogoproto"
import _ "github.com/gogo/googleapis/google/api"
import _ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger/options"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = golang_proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type PostSpansRequest struct {
	Batch jaeger_api_v2.Batch `protobuf:"bytes,1,opt,name=batch" json:"batch"`
}

func (m *PostSpansRequest) Reset()                    { *m = PostSpansRequest{} }
func (m *PostSpansRequest) String() string            { return proto.CompactTextString(m) }
func (*PostSpansRequest) ProtoMessage()               {}
func (*PostSpansRequest) Descriptor() ([]byte, []int) { return fileDescriptorCollector, []int{0} }

func (m *PostSpansRequest) GetBatch() jaeger_api_v2.Batch {
	if m != nil {
		return m.Batch
	}
	return jaeger_api_v2.Batch{}
}

type PostSpansResponse struct {
}

func (m *PostSpansResponse) Reset()                    { *m = PostSpansResponse{} }
func (m *PostSpansResponse) String() string            { return proto.CompactTextString(m) }
func (*PostSpansResponse) ProtoMessage()               {}
func (*PostSpansResponse) Descriptor() ([]byte, []int) { return fileDescriptorCollector, []int{1} }

func init() {
	proto.RegisterType((*PostSpansRequest)(nil), "jaeger.api_v2.PostSpansRequest")
	golang_proto.RegisterType((*PostSpansRequest)(nil), "jaeger.api_v2.PostSpansRequest")
	proto.RegisterType((*PostSpansResponse)(nil), "jaeger.api_v2.PostSpansResponse")
	golang_proto.RegisterType((*PostSpansResponse)(nil), "jaeger.api_v2.PostSpansResponse")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Client API for CollectorService service

type CollectorServiceClient interface {
	PostSpans(ctx context.Context, in *PostSpansRequest, opts ...grpc.CallOption) (*PostSpansResponse, error)
}

type collectorServiceClient struct {
	cc *grpc.ClientConn
}

func NewCollectorServiceClient(cc *grpc.ClientConn) CollectorServiceClient {
	return &collectorServiceClient{cc}
}

func (c *collectorServiceClient) PostSpans(ctx context.Context, in *PostSpansRequest, opts ...grpc.CallOption) (*PostSpansResponse, error) {
	out := new(PostSpansResponse)
	err := grpc.Invoke(ctx, "/jaeger.api_v2.CollectorService/PostSpans", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for CollectorService service

type CollectorServiceServer interface {
	PostSpans(context.Context, *PostSpansRequest) (*PostSpansResponse, error)
}

func RegisterCollectorServiceServer(s *grpc.Server, srv CollectorServiceServer) {
	s.RegisterService(&_CollectorService_serviceDesc, srv)
}

func _CollectorService_PostSpans_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PostSpansRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CollectorServiceServer).PostSpans(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jaeger.api_v2.CollectorService/PostSpans",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CollectorServiceServer).PostSpans(ctx, req.(*PostSpansRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _CollectorService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "jaeger.api_v2.CollectorService",
	HandlerType: (*CollectorServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "PostSpans",
			Handler:    _CollectorService_PostSpans_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "model/proto/api_v2/collector.proto",
}

func (m *PostSpansRequest) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *PostSpansRequest) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0xa
	i++
	i = encodeVarintCollector(dAtA, i, uint64(m.Batch.Size()))
	n1, err := m.Batch.MarshalTo(dAtA[i:])
	if err != nil {
		return 0, err
	}
	i += n1
	return i, nil
}

func (m *PostSpansResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *PostSpansResponse) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	return i, nil
}

func encodeFixed64Collector(dAtA []byte, offset int, v uint64) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	dAtA[offset+4] = uint8(v >> 32)
	dAtA[offset+5] = uint8(v >> 40)
	dAtA[offset+6] = uint8(v >> 48)
	dAtA[offset+7] = uint8(v >> 56)
	return offset + 8
}
func encodeFixed32Collector(dAtA []byte, offset int, v uint32) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	return offset + 4
}
func encodeVarintCollector(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *PostSpansRequest) Size() (n int) {
	var l int
	_ = l
	l = m.Batch.Size()
	n += 1 + l + sovCollector(uint64(l))
	return n
}

func (m *PostSpansResponse) Size() (n int) {
	var l int
	_ = l
	return n
}

func sovCollector(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozCollector(x uint64) (n int) {
	return sovCollector(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *PostSpansRequest) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowCollector
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: PostSpansRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: PostSpansRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Batch", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowCollector
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthCollector
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Batch.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipCollector(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthCollector
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *PostSpansResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowCollector
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: PostSpansResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: PostSpansResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipCollector(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthCollector
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipCollector(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowCollector
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowCollector
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowCollector
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthCollector
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowCollector
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipCollector(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthCollector = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowCollector   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("model/proto/api_v2/collector.proto", fileDescriptorCollector) }
func init() { golang_proto.RegisterFile("model/proto/api_v2/collector.proto", fileDescriptorCollector) }

var fileDescriptorCollector = []byte{
	// 341 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x90, 0xc1, 0x4e, 0xfa, 0x40,
	0x10, 0xc6, 0xff, 0xcb, 0x5f, 0x48, 0x5c, 0x42, 0x82, 0x95, 0x44, 0x42, 0x4c, 0x21, 0xbd, 0x68,
	0x88, 0x74, 0xb1, 0xc6, 0x0b, 0x37, 0xaa, 0x17, 0x3d, 0x11, 0xb8, 0x79, 0x31, 0x4b, 0xdd, 0x6c,
	0x6b, 0x4a, 0xa7, 0xee, 0x2e, 0xf5, 0xe2, 0xc9, 0x47, 0xd0, 0x17, 0xf2, 0xc8, 0xd1, 0xc4, 0xbb,
	0x31, 0xe8, 0x83, 0x98, 0xee, 0x56, 0x23, 0x18, 0x4f, 0xd3, 0xce, 0xf7, 0x9b, 0x6f, 0xbe, 0x1d,
	0xec, 0xcc, 0xe0, 0x8a, 0xc5, 0x24, 0x15, 0xa0, 0x80, 0xd0, 0x34, 0xba, 0xcc, 0x3c, 0x12, 0x40,
	0x1c, 0xb3, 0x40, 0x81, 0x70, 0x75, 0xdb, 0xaa, 0x5d, 0x53, 0xc6, 0x99, 0x70, 0x8d, 0xdc, 0xaa,
	0xea, 0x11, 0xa3, 0xb5, 0x1a, 0x1c, 0x38, 0x98, 0xe9, 0xfc, 0xab, 0xe8, 0xee, 0x72, 0x00, 0x1e,
	0xb3, 0xdc, 0x90, 0xd0, 0x24, 0x01, 0x45, 0x55, 0x04, 0x89, 0x2c, 0xd4, 0x03, 0x5d, 0x82, 0x1e,
	0x67, 0x49, 0x4f, 0xde, 0x52, 0xce, 0x99, 0x20, 0x90, 0x6a, 0xe2, 0x37, 0xed, 0x9c, 0xe2, 0xfa,
	0x08, 0xa4, 0x9a, 0xa4, 0x34, 0x91, 0x63, 0x76, 0x33, 0x67, 0x52, 0x59, 0x7d, 0x5c, 0x9e, 0x52,
	0x15, 0x84, 0x4d, 0xd4, 0x41, 0xfb, 0x55, 0xaf, 0xe1, 0xae, 0x24, 0x74, 0xfd, 0x5c, 0xf3, 0x37,
	0x16, 0xaf, 0xed, 0x7f, 0x63, 0x03, 0x3a, 0xdb, 0x78, 0xeb, 0x87, 0x8b, 0x4c, 0x21, 0x91, 0xcc,
	0xbb, 0xc3, 0xf5, 0x93, 0xaf, 0xb7, 0x4e, 0x98, 0xc8, 0xa2, 0x80, 0x59, 0x21, 0xde, 0xfc, 0x06,
	0xad, 0xf6, 0x9a, 0xf1, 0x7a, 0x90, 0x56, 0xe7, 0x6f, 0xc0, 0xec, 0x70, 0x9a, 0xf7, 0x2f, 0x1f,
	0x8f, 0x25, 0xcb, 0xa9, 0xe9, 0x63, 0x64, 0x1e, 0x91, 0xb9, 0x3c, 0x40, 0x5d, 0x3f, 0x7b, 0x18,
	0xfa, 0x56, 0xd9, 0xfb, 0x7f, 0xe8, 0xf6, 0xbb, 0x25, 0x54, 0x12, 0xc7, 0x18, 0x9f, 0x6b, 0xb3,
	0xce, 0x70, 0x74, 0x66, 0xed, 0x85, 0x4a, 0xa5, 0x72, 0x40, 0x08, 0x8f, 0x54, 0x38, 0x9f, 0xba,
	0x01, 0xcc, 0x88, 0xd9, 0xa5, 0x04, 0x0d, 0xa2, 0x84, 0x17, 0x7f, 0x8b, 0xa5, 0x8d, 0x9e, 0x97,
	0x36, 0x7a, 0x5b, 0xda, 0xe8, 0xe9, 0xdd, 0x46, 0x78, 0x27, 0x02, 0x77, 0x05, 0x2c, 0xb2, 0x5d,
	0x54, 0x4c, 0x9d, 0x56, 0xf4, 0x5d, 0x8f, 0x3e, 0x03, 0x00, 0x00, 0xff, 0xff, 0xd0, 0xef, 0x4f,
	0x5d, 0xfb, 0x01, 0x00, 0x00,
}
