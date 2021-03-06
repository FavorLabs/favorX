// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: netrelay.proto

package pb

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	io "io"
	math "math"
	math_bits "math/bits"
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

type RelayHttpReq struct {
	Url     string `protobuf:"bytes,1,opt,name=Url,proto3" json:"Url,omitempty"`
	Method  []byte `protobuf:"bytes,2,opt,name=Method,proto3" json:"Method,omitempty"`
	Header  []byte `protobuf:"bytes,3,opt,name=Header,proto3" json:"Header,omitempty"`
	Body    []byte `protobuf:"bytes,4,opt,name=Body,proto3" json:"Body,omitempty"`
	Timeout int64  `protobuf:"varint,5,opt,name=Timeout,proto3" json:"Timeout,omitempty"`
}

func (m *RelayHttpReq) Reset()         { *m = RelayHttpReq{} }
func (m *RelayHttpReq) String() string { return proto.CompactTextString(m) }
func (*RelayHttpReq) ProtoMessage()    {}
func (*RelayHttpReq) Descriptor() ([]byte, []int) {
	return fileDescriptor_2ede289ea5d6ac4f, []int{0}
}
func (m *RelayHttpReq) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *RelayHttpReq) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_RelayHttpReq.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *RelayHttpReq) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RelayHttpReq.Merge(m, src)
}
func (m *RelayHttpReq) XXX_Size() int {
	return m.Size()
}
func (m *RelayHttpReq) XXX_DiscardUnknown() {
	xxx_messageInfo_RelayHttpReq.DiscardUnknown(m)
}

var xxx_messageInfo_RelayHttpReq proto.InternalMessageInfo

func (m *RelayHttpReq) GetUrl() string {
	if m != nil {
		return m.Url
	}
	return ""
}

func (m *RelayHttpReq) GetMethod() []byte {
	if m != nil {
		return m.Method
	}
	return nil
}

func (m *RelayHttpReq) GetHeader() []byte {
	if m != nil {
		return m.Header
	}
	return nil
}

func (m *RelayHttpReq) GetBody() []byte {
	if m != nil {
		return m.Body
	}
	return nil
}

func (m *RelayHttpReq) GetTimeout() int64 {
	if m != nil {
		return m.Timeout
	}
	return 0
}

type RelayHttpResp struct {
	Status int32  `protobuf:"varint,1,opt,name=Status,proto3" json:"Status,omitempty"`
	Header []byte `protobuf:"bytes,2,opt,name=Header,proto3" json:"Header,omitempty"`
	Body   []byte `protobuf:"bytes,3,opt,name=Body,proto3" json:"Body,omitempty"`
}

func (m *RelayHttpResp) Reset()         { *m = RelayHttpResp{} }
func (m *RelayHttpResp) String() string { return proto.CompactTextString(m) }
func (*RelayHttpResp) ProtoMessage()    {}
func (*RelayHttpResp) Descriptor() ([]byte, []int) {
	return fileDescriptor_2ede289ea5d6ac4f, []int{1}
}
func (m *RelayHttpResp) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *RelayHttpResp) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_RelayHttpResp.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *RelayHttpResp) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RelayHttpResp.Merge(m, src)
}
func (m *RelayHttpResp) XXX_Size() int {
	return m.Size()
}
func (m *RelayHttpResp) XXX_DiscardUnknown() {
	xxx_messageInfo_RelayHttpResp.DiscardUnknown(m)
}

var xxx_messageInfo_RelayHttpResp proto.InternalMessageInfo

func (m *RelayHttpResp) GetStatus() int32 {
	if m != nil {
		return m.Status
	}
	return 0
}

func (m *RelayHttpResp) GetHeader() []byte {
	if m != nil {
		return m.Header
	}
	return nil
}

func (m *RelayHttpResp) GetBody() []byte {
	if m != nil {
		return m.Body
	}
	return nil
}

func init() {
	proto.RegisterType((*RelayHttpReq)(nil), "netrelayFavorX.RelayHttpReq")
	proto.RegisterType((*RelayHttpResp)(nil), "netrelayFavorX.RelayHttpResp")
}

func init() { proto.RegisterFile("netrelay.proto", fileDescriptor_2ede289ea5d6ac4f) }

var fileDescriptor_2ede289ea5d6ac4f = []byte{
	// 212 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0xcb, 0x4b, 0x2d, 0x29,
	0x4a, 0xcd, 0x49, 0xac, 0xd4, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x82, 0xf3, 0xdd, 0x12, 0xcb,
	0xf2, 0x8b, 0x22, 0x94, 0xea, 0xb8, 0x78, 0x82, 0x40, 0x5c, 0x8f, 0x92, 0x92, 0x82, 0xa0, 0xd4,
	0x42, 0x21, 0x01, 0x2e, 0xe6, 0xd0, 0xa2, 0x1c, 0x09, 0x46, 0x05, 0x46, 0x0d, 0xce, 0x20, 0x10,
	0x53, 0x48, 0x8c, 0x8b, 0xcd, 0x37, 0xb5, 0x24, 0x23, 0x3f, 0x45, 0x82, 0x49, 0x81, 0x51, 0x83,
	0x27, 0x08, 0xca, 0x03, 0x89, 0x7b, 0xa4, 0x26, 0xa6, 0xa4, 0x16, 0x49, 0x30, 0x43, 0xc4, 0x21,
	0x3c, 0x21, 0x21, 0x2e, 0x16, 0xa7, 0xfc, 0x94, 0x4a, 0x09, 0x16, 0xb0, 0x28, 0x98, 0x2d, 0x24,
	0xc1, 0xc5, 0x1e, 0x92, 0x99, 0x9b, 0x9a, 0x5f, 0x5a, 0x22, 0xc1, 0xaa, 0xc0, 0xa8, 0xc1, 0x1c,
	0x04, 0xe3, 0x2a, 0x05, 0x73, 0xf1, 0x22, 0xd9, 0x5f, 0x5c, 0x00, 0x32, 0x36, 0xb8, 0x24, 0xb1,
	0xa4, 0xb4, 0x18, 0xec, 0x06, 0xd6, 0x20, 0x28, 0x0f, 0xc9, 0x3a, 0x26, 0xac, 0xd6, 0x31, 0x23,
	0xac, 0x73, 0x92, 0x39, 0xf1, 0x48, 0x8e, 0xf1, 0xc2, 0x23, 0x39, 0xc6, 0x07, 0x8f, 0xe4, 0x18,
	0x27, 0x3c, 0x96, 0x63, 0xb8, 0xf0, 0x58, 0x8e, 0xe1, 0xc6, 0x63, 0x39, 0x86, 0x28, 0xa6, 0x82,
	0xa4, 0x24, 0x36, 0x70, 0x48, 0x18, 0x03, 0x02, 0x00, 0x00, 0xff, 0xff, 0x11, 0x0d, 0x10, 0xc9,
	0x1b, 0x01, 0x00, 0x00,
}

func (m *RelayHttpReq) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *RelayHttpReq) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *RelayHttpReq) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Timeout != 0 {
		i = encodeVarintNetrelay(dAtA, i, uint64(m.Timeout))
		i--
		dAtA[i] = 0x28
	}
	if len(m.Body) > 0 {
		i -= len(m.Body)
		copy(dAtA[i:], m.Body)
		i = encodeVarintNetrelay(dAtA, i, uint64(len(m.Body)))
		i--
		dAtA[i] = 0x22
	}
	if len(m.Header) > 0 {
		i -= len(m.Header)
		copy(dAtA[i:], m.Header)
		i = encodeVarintNetrelay(dAtA, i, uint64(len(m.Header)))
		i--
		dAtA[i] = 0x1a
	}
	if len(m.Method) > 0 {
		i -= len(m.Method)
		copy(dAtA[i:], m.Method)
		i = encodeVarintNetrelay(dAtA, i, uint64(len(m.Method)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.Url) > 0 {
		i -= len(m.Url)
		copy(dAtA[i:], m.Url)
		i = encodeVarintNetrelay(dAtA, i, uint64(len(m.Url)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *RelayHttpResp) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *RelayHttpResp) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *RelayHttpResp) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Body) > 0 {
		i -= len(m.Body)
		copy(dAtA[i:], m.Body)
		i = encodeVarintNetrelay(dAtA, i, uint64(len(m.Body)))
		i--
		dAtA[i] = 0x1a
	}
	if len(m.Header) > 0 {
		i -= len(m.Header)
		copy(dAtA[i:], m.Header)
		i = encodeVarintNetrelay(dAtA, i, uint64(len(m.Header)))
		i--
		dAtA[i] = 0x12
	}
	if m.Status != 0 {
		i = encodeVarintNetrelay(dAtA, i, uint64(m.Status))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintNetrelay(dAtA []byte, offset int, v uint64) int {
	offset -= sovNetrelay(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *RelayHttpReq) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Url)
	if l > 0 {
		n += 1 + l + sovNetrelay(uint64(l))
	}
	l = len(m.Method)
	if l > 0 {
		n += 1 + l + sovNetrelay(uint64(l))
	}
	l = len(m.Header)
	if l > 0 {
		n += 1 + l + sovNetrelay(uint64(l))
	}
	l = len(m.Body)
	if l > 0 {
		n += 1 + l + sovNetrelay(uint64(l))
	}
	if m.Timeout != 0 {
		n += 1 + sovNetrelay(uint64(m.Timeout))
	}
	return n
}

func (m *RelayHttpResp) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Status != 0 {
		n += 1 + sovNetrelay(uint64(m.Status))
	}
	l = len(m.Header)
	if l > 0 {
		n += 1 + l + sovNetrelay(uint64(l))
	}
	l = len(m.Body)
	if l > 0 {
		n += 1 + l + sovNetrelay(uint64(l))
	}
	return n
}

func sovNetrelay(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozNetrelay(x uint64) (n int) {
	return sovNetrelay(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *RelayHttpReq) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetrelay
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
			return fmt.Errorf("proto: RelayHttpReq: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: RelayHttpReq: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Url", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthNetrelay
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthNetrelay
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Url = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Method", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetrelay
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetrelay
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Method = append(m.Method[:0], dAtA[iNdEx:postIndex]...)
			if m.Method == nil {
				m.Method = []byte{}
			}
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Header", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetrelay
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetrelay
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Header = append(m.Header[:0], dAtA[iNdEx:postIndex]...)
			if m.Header == nil {
				m.Header = []byte{}
			}
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Body", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetrelay
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetrelay
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Body = append(m.Body[:0], dAtA[iNdEx:postIndex]...)
			if m.Body == nil {
				m.Body = []byte{}
			}
			iNdEx = postIndex
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Timeout", wireType)
			}
			m.Timeout = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Timeout |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipNetrelay(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetrelay
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
func (m *RelayHttpResp) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetrelay
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
			return fmt.Errorf("proto: RelayHttpResp: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: RelayHttpResp: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Status", wireType)
			}
			m.Status = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Status |= int32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Header", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetrelay
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetrelay
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Header = append(m.Header[:0], dAtA[iNdEx:postIndex]...)
			if m.Header == nil {
				m.Header = []byte{}
			}
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Body", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetrelay
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetrelay
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Body = append(m.Body[:0], dAtA[iNdEx:postIndex]...)
			if m.Body == nil {
				m.Body = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipNetrelay(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetrelay
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
func skipNetrelay(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowNetrelay
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
					return 0, ErrIntOverflowNetrelay
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowNetrelay
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
				return 0, ErrInvalidLengthNetrelay
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupNetrelay
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthNetrelay
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthNetrelay        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowNetrelay          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupNetrelay = fmt.Errorf("proto: unexpected end of group")
)
