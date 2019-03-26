// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: api_v2/collector.proto

package api_v2

import (
	context "context"
	fmt "fmt"
	io "io"
	math "math"

	_ "github.com/gogo/googleapis/google/api"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	golang_proto "github.com/golang/protobuf/proto"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger/options"
	grpc "google.golang.org/grpc"

	model "github.com/jaegertracing/jaeger/model"
)

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
	Batch                model.Batch `protobuf:"bytes,1,opt,name=batch,proto3" json:"batch"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *PostSpansRequest) Reset()         { *m = PostSpansRequest{} }
func (m *PostSpansRequest) String() string { return proto.CompactTextString(m) }
func (*PostSpansRequest) ProtoMessage()    {}
func (*PostSpansRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_495529cb13d121cf, []int{0}
}
func (m *PostSpansRequest) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *PostSpansRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_PostSpansRequest.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *PostSpansRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PostSpansRequest.Merge(m, src)
}
func (m *PostSpansRequest) XXX_Size() int {
	return m.Size()
}
func (m *PostSpansRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_PostSpansRequest.DiscardUnknown(m)
}

var xxx_messageInfo_PostSpansRequest proto.InternalMessageInfo

func (m *PostSpansRequest) GetBatch() model.Batch {
	if m != nil {
		return m.Batch
	}
	return model.Batch{}
}

type PostSpansResponse struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PostSpansResponse) Reset()         { *m = PostSpansResponse{} }
func (m *PostSpansResponse) String() string { return proto.CompactTextString(m) }
func (*PostSpansResponse) ProtoMessage()    {}
func (*PostSpansResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_495529cb13d121cf, []int{1}
}
func (m *PostSpansResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *PostSpansResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_PostSpansResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *PostSpansResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PostSpansResponse.Merge(m, src)
}
func (m *PostSpansResponse) XXX_Size() int {
	return m.Size()
}
func (m *PostSpansResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_PostSpansResponse.DiscardUnknown(m)
}

var xxx_messageInfo_PostSpansResponse proto.InternalMessageInfo

func init() {
	proto.RegisterType((*PostSpansRequest)(nil), "jaeger.api_v2.PostSpansRequest")
	golang_proto.RegisterType((*PostSpansRequest)(nil), "jaeger.api_v2.PostSpansRequest")
	proto.RegisterType((*PostSpansResponse)(nil), "jaeger.api_v2.PostSpansResponse")
	golang_proto.RegisterType((*PostSpansResponse)(nil), "jaeger.api_v2.PostSpansResponse")
}

func init() { proto.RegisterFile("api_v2/collector.proto", fileDescriptor_495529cb13d121cf) }
func init() { golang_proto.RegisterFile("api_v2/collector.proto", fileDescriptor_495529cb13d121cf) }

var fileDescriptor_495529cb13d121cf = []byte{
	// 336 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x90, 0x31, 0x4f, 0xc2, 0x40,
	0x14, 0xc7, 0x3d, 0x14, 0x12, 0x8f, 0x90, 0x60, 0x25, 0x4a, 0x88, 0x29, 0xa4, 0x8b, 0x86, 0x48,
	0x0f, 0x6b, 0x5c, 0xd8, 0xa8, 0x2e, 0x3a, 0x11, 0xd8, 0x5c, 0xcc, 0x51, 0x2f, 0xd7, 0x9a, 0x72,
	0xef, 0xbc, 0x3b, 0xea, 0xe2, 0xe4, 0x47, 0xd0, 0x2f, 0xe4, 0xc8, 0x68, 0xe2, 0x6e, 0x0c, 0xfa,
	0x41, 0x0c, 0x6d, 0x35, 0x82, 0x71, 0x7a, 0xef, 0xee, 0xff, 0xcb, 0xff, 0xfd, 0xdf, 0xc3, 0x3b,
	0x54, 0x46, 0x57, 0x89, 0x47, 0x02, 0x88, 0x63, 0x16, 0x18, 0x50, 0xae, 0x54, 0x60, 0xc0, 0xaa,
	0xdc, 0x50, 0xc6, 0x99, 0x72, 0x33, 0xb9, 0x51, 0x9e, 0xc0, 0x35, 0x8b, 0x33, 0xad, 0x51, 0xe3,
	0xc0, 0x21, 0x6d, 0xc9, 0xa2, 0xcb, 0x7f, 0xf7, 0x38, 0x00, 0x8f, 0x19, 0xa1, 0x32, 0x22, 0x54,
	0x08, 0x30, 0xd4, 0x44, 0x20, 0x74, 0xae, 0x1e, 0xa6, 0x25, 0xe8, 0x70, 0x26, 0x3a, 0xfa, 0x8e,
	0x72, 0xce, 0x14, 0x01, 0x99, 0x12, 0x7f, 0x69, 0xe7, 0x0c, 0x57, 0x07, 0xa0, 0xcd, 0x48, 0x52,
	0xa1, 0x87, 0xec, 0x76, 0xca, 0xb4, 0xb1, 0xba, 0xb8, 0x38, 0xa6, 0x26, 0x08, 0xeb, 0xa8, 0x85,
	0x0e, 0xca, 0x5e, 0xcd, 0x5d, 0x4a, 0xe8, 0xfa, 0x0b, 0xcd, 0xdf, 0x98, 0xbd, 0x35, 0xd7, 0x86,
	0x19, 0xe8, 0x6c, 0xe3, 0xad, 0x5f, 0x2e, 0x5a, 0x82, 0xd0, 0xcc, 0xbb, 0xc7, 0xd5, 0xd3, 0xef,
	0x5d, 0x47, 0x4c, 0x25, 0x51, 0xc0, 0xac, 0x10, 0x6f, 0xfe, 0x80, 0x56, 0x73, 0xc5, 0x78, 0x35,
	0x48, 0xa3, 0xf5, 0x3f, 0x90, 0xcd, 0x70, 0xea, 0x0f, 0xaf, 0x9f, 0x4f, 0x05, 0xcb, 0xa9, 0xa4,
	0xc7, 0x48, 0x3c, 0xa2, 0x17, 0x72, 0x0f, 0xb5, 0xfd, 0xe4, 0xb1, 0xef, 0x5b, 0x45, 0x6f, 0xfd,
	0xc8, 0xed, 0xb6, 0x0b, 0xa8, 0xa0, 0x4e, 0x30, 0xbe, 0x48, 0xcd, 0x5a, 0xfd, 0xc1, 0xb9, 0xb5,
	0x1f, 0x1a, 0x23, 0x75, 0x8f, 0x10, 0x1e, 0x99, 0x70, 0x3a, 0x76, 0x03, 0x98, 0x90, 0x6c, 0x96,
	0x51, 0x34, 0x88, 0x04, 0xcf, 0x5f, 0xb3, 0xb9, 0x8d, 0x5e, 0xe6, 0x36, 0x7a, 0x9f, 0xdb, 0xe8,
	0xf9, 0xc3, 0x46, 0x78, 0x37, 0x02, 0x77, 0x09, 0xcc, 0xb3, 0x5d, 0x96, 0xb2, 0x3a, 0x2e, 0xa5,
	0x77, 0x3d, 0xfe, 0x0a, 0x00, 0x00, 0xff, 0xff, 0x5b, 0xe8, 0xa6, 0x4c, 0xef, 0x01, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// CollectorServiceClient is the client API for CollectorService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
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
	err := c.cc.Invoke(ctx, "/jaeger.api_v2.CollectorService/PostSpans", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CollectorServiceServer is the server API for CollectorService service.
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
	Metadata: "api_v2/collector.proto",
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
	if m.XXX_unrecognized != nil {
		i += copy(dAtA[i:], m.XXX_unrecognized)
	}
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
	if m.XXX_unrecognized != nil {
		i += copy(dAtA[i:], m.XXX_unrecognized)
	}
	return i, nil
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
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.Batch.Size()
	n += 1 + l + sovCollector(uint64(l))
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *PostSpansResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
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
			wire |= uint64(b&0x7F) << shift
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
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthCollector
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthCollector
			}
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
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthCollector
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
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
			wire |= uint64(b&0x7F) << shift
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
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthCollector
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
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
			if length < 0 {
				return 0, ErrInvalidLengthCollector
			}
			iNdEx += length
			if iNdEx < 0 {
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
				if iNdEx < 0 {
					return 0, ErrInvalidLengthCollector
				}
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
