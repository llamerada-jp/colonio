// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        (unknown)
// source: api/colonio/v1alpha/seed.proto

package v1alpha

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type SignalOfferType int32

const (
	// buf:lint:ignore ENUM_ZERO_VALUE_SUFFIX
	SignalOfferType_SIGNAL_OFFER_TYPE_EXPLICIT SignalOfferType = 0
	SignalOfferType_SIGNAL_OFFER_TYPE_NEXT     SignalOfferType = 1
)

// Enum value maps for SignalOfferType.
var (
	SignalOfferType_name = map[int32]string{
		0: "SIGNAL_OFFER_TYPE_EXPLICIT",
		1: "SIGNAL_OFFER_TYPE_NEXT",
	}
	SignalOfferType_value = map[string]int32{
		"SIGNAL_OFFER_TYPE_EXPLICIT": 0,
		"SIGNAL_OFFER_TYPE_NEXT":     1,
	}
)

func (x SignalOfferType) Enum() *SignalOfferType {
	p := new(SignalOfferType)
	*p = x
	return p
}

func (x SignalOfferType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (SignalOfferType) Descriptor() protoreflect.EnumDescriptor {
	return file_api_colonio_v1alpha_seed_proto_enumTypes[0].Descriptor()
}

func (SignalOfferType) Type() protoreflect.EnumType {
	return &file_api_colonio_v1alpha_seed_proto_enumTypes[0]
}

func (x SignalOfferType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use SignalOfferType.Descriptor instead.
func (SignalOfferType) EnumDescriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{0}
}

type Signal struct {
	state     protoimpl.MessageState `protogen:"open.v1"`
	DstNodeId *NodeID                `protobuf:"bytes,1,opt,name=dst_node_id,json=dstNodeId,proto3" json:"dst_node_id,omitempty"`
	SrcNodeId *NodeID                `protobuf:"bytes,2,opt,name=src_node_id,json=srcNodeId,proto3" json:"src_node_id,omitempty"`
	// Types that are valid to be assigned to Content:
	//
	//	*Signal_Offer
	//	*Signal_Answer
	//	*Signal_Ice
	Content       isSignal_Content `protobuf_oneof:"content"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Signal) Reset() {
	*x = Signal{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Signal) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Signal) ProtoMessage() {}

func (x *Signal) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Signal.ProtoReflect.Descriptor instead.
func (*Signal) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{0}
}

func (x *Signal) GetDstNodeId() *NodeID {
	if x != nil {
		return x.DstNodeId
	}
	return nil
}

func (x *Signal) GetSrcNodeId() *NodeID {
	if x != nil {
		return x.SrcNodeId
	}
	return nil
}

func (x *Signal) GetContent() isSignal_Content {
	if x != nil {
		return x.Content
	}
	return nil
}

func (x *Signal) GetOffer() *SignalOffer {
	if x != nil {
		if x, ok := x.Content.(*Signal_Offer); ok {
			return x.Offer
		}
	}
	return nil
}

func (x *Signal) GetAnswer() *SignalAnswer {
	if x != nil {
		if x, ok := x.Content.(*Signal_Answer); ok {
			return x.Answer
		}
	}
	return nil
}

func (x *Signal) GetIce() *SignalICE {
	if x != nil {
		if x, ok := x.Content.(*Signal_Ice); ok {
			return x.Ice
		}
	}
	return nil
}

type isSignal_Content interface {
	isSignal_Content()
}

type Signal_Offer struct {
	Offer *SignalOffer `protobuf:"bytes,3,opt,name=offer,proto3,oneof"`
}

type Signal_Answer struct {
	Answer *SignalAnswer `protobuf:"bytes,4,opt,name=answer,proto3,oneof"`
}

type Signal_Ice struct {
	Ice *SignalICE `protobuf:"bytes,5,opt,name=ice,proto3,oneof"`
}

func (*Signal_Offer) isSignal_Content() {}

func (*Signal_Answer) isSignal_Content() {}

func (*Signal_Ice) isSignal_Content() {}

type SignalOffer struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	OfferId       uint32                 `protobuf:"varint,1,opt,name=offer_id,json=offerId,proto3" json:"offer_id,omitempty"`
	Type          SignalOfferType        `protobuf:"varint,2,opt,name=type,proto3,enum=api.colonio.v1alpha.SignalOfferType" json:"type,omitempty"`
	Sdp           string                 `protobuf:"bytes,3,opt,name=sdp,proto3" json:"sdp,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SignalOffer) Reset() {
	*x = SignalOffer{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SignalOffer) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignalOffer) ProtoMessage() {}

func (x *SignalOffer) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignalOffer.ProtoReflect.Descriptor instead.
func (*SignalOffer) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{1}
}

func (x *SignalOffer) GetOfferId() uint32 {
	if x != nil {
		return x.OfferId
	}
	return 0
}

func (x *SignalOffer) GetType() SignalOfferType {
	if x != nil {
		return x.Type
	}
	return SignalOfferType_SIGNAL_OFFER_TYPE_EXPLICIT
}

func (x *SignalOffer) GetSdp() string {
	if x != nil {
		return x.Sdp
	}
	return ""
}

type SignalAnswer struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	OfferId       uint32                 `protobuf:"varint,1,opt,name=offer_id,json=offerId,proto3" json:"offer_id,omitempty"`
	Status        uint32                 `protobuf:"varint,2,opt,name=status,proto3" json:"status,omitempty"`
	Sdp           string                 `protobuf:"bytes,3,opt,name=sdp,proto3" json:"sdp,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SignalAnswer) Reset() {
	*x = SignalAnswer{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SignalAnswer) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignalAnswer) ProtoMessage() {}

func (x *SignalAnswer) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignalAnswer.ProtoReflect.Descriptor instead.
func (*SignalAnswer) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{2}
}

func (x *SignalAnswer) GetOfferId() uint32 {
	if x != nil {
		return x.OfferId
	}
	return 0
}

func (x *SignalAnswer) GetStatus() uint32 {
	if x != nil {
		return x.Status
	}
	return 0
}

func (x *SignalAnswer) GetSdp() string {
	if x != nil {
		return x.Sdp
	}
	return ""
}

type SignalICE struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	OfferId       uint32                 `protobuf:"varint,1,opt,name=offer_id,json=offerId,proto3" json:"offer_id,omitempty"`
	Ices          []string               `protobuf:"bytes,2,rep,name=ices,proto3" json:"ices,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SignalICE) Reset() {
	*x = SignalICE{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SignalICE) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignalICE) ProtoMessage() {}

func (x *SignalICE) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignalICE.ProtoReflect.Descriptor instead.
func (*SignalICE) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{3}
}

func (x *SignalICE) GetOfferId() uint32 {
	if x != nil {
		return x.OfferId
	}
	return 0
}

func (x *SignalICE) GetIces() []string {
	if x != nil {
		return x.Ices
	}
	return nil
}

type AssignNodeRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *AssignNodeRequest) Reset() {
	*x = AssignNodeRequest{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AssignNodeRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AssignNodeRequest) ProtoMessage() {}

func (x *AssignNodeRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AssignNodeRequest.ProtoReflect.Descriptor instead.
func (*AssignNodeRequest) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{4}
}

type AssignNodeResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	NodeId        *NodeID                `protobuf:"bytes,1,opt,name=node_id,json=nodeId,proto3" json:"node_id,omitempty"`
	IsAlone       bool                   `protobuf:"varint,2,opt,name=is_alone,json=isAlone,proto3" json:"is_alone,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *AssignNodeResponse) Reset() {
	*x = AssignNodeResponse{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AssignNodeResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AssignNodeResponse) ProtoMessage() {}

func (x *AssignNodeResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AssignNodeResponse.ProtoReflect.Descriptor instead.
func (*AssignNodeResponse) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{5}
}

func (x *AssignNodeResponse) GetNodeId() *NodeID {
	if x != nil {
		return x.NodeId
	}
	return nil
}

func (x *AssignNodeResponse) GetIsAlone() bool {
	if x != nil {
		return x.IsAlone
	}
	return false
}

type UnassignNodeRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *UnassignNodeRequest) Reset() {
	*x = UnassignNodeRequest{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UnassignNodeRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UnassignNodeRequest) ProtoMessage() {}

func (x *UnassignNodeRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UnassignNodeRequest.ProtoReflect.Descriptor instead.
func (*UnassignNodeRequest) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{6}
}

type UnassignNodeResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *UnassignNodeResponse) Reset() {
	*x = UnassignNodeResponse{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UnassignNodeResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UnassignNodeResponse) ProtoMessage() {}

func (x *UnassignNodeResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UnassignNodeResponse.ProtoReflect.Descriptor instead.
func (*UnassignNodeResponse) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{7}
}

type SendSignalRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Signal        *Signal                `protobuf:"bytes,1,opt,name=signal,proto3" json:"signal,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SendSignalRequest) Reset() {
	*x = SendSignalRequest{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SendSignalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SendSignalRequest) ProtoMessage() {}

func (x *SendSignalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SendSignalRequest.ProtoReflect.Descriptor instead.
func (*SendSignalRequest) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{8}
}

func (x *SendSignalRequest) GetSignal() *Signal {
	if x != nil {
		return x.Signal
	}
	return nil
}

type SendSignalResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	IsAlone       bool                   `protobuf:"varint,1,opt,name=is_alone,json=isAlone,proto3" json:"is_alone,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SendSignalResponse) Reset() {
	*x = SendSignalResponse{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SendSignalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SendSignalResponse) ProtoMessage() {}

func (x *SendSignalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SendSignalResponse.ProtoReflect.Descriptor instead.
func (*SendSignalResponse) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{9}
}

func (x *SendSignalResponse) GetIsAlone() bool {
	if x != nil {
		return x.IsAlone
	}
	return false
}

type PollSignalRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PollSignalRequest) Reset() {
	*x = PollSignalRequest{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[10]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PollSignalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PollSignalRequest) ProtoMessage() {}

func (x *PollSignalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[10]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PollSignalRequest.ProtoReflect.Descriptor instead.
func (*PollSignalRequest) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{10}
}

type PollSignalResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Signals       []*Signal              `protobuf:"bytes,1,rep,name=signals,proto3" json:"signals,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PollSignalResponse) Reset() {
	*x = PollSignalResponse{}
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[11]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PollSignalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PollSignalResponse) ProtoMessage() {}

func (x *PollSignalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_colonio_v1alpha_seed_proto_msgTypes[11]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PollSignalResponse.ProtoReflect.Descriptor instead.
func (*PollSignalResponse) Descriptor() ([]byte, []int) {
	return file_api_colonio_v1alpha_seed_proto_rawDescGZIP(), []int{11}
}

func (x *PollSignalResponse) GetSignals() []*Signal {
	if x != nil {
		return x.Signals
	}
	return nil
}

var File_api_colonio_v1alpha_seed_proto protoreflect.FileDescriptor

const file_api_colonio_v1alpha_seed_proto_rawDesc = "" +
	"\n" +
	"\x1eapi/colonio/v1alpha/seed.proto\x12\x13api.colonio.v1alpha\x1a!api/colonio/v1alpha/colonio.proto\"\xb8\x02\n" +
	"\x06Signal\x12;\n" +
	"\vdst_node_id\x18\x01 \x01(\v2\x1b.api.colonio.v1alpha.NodeIDR\tdstNodeId\x12;\n" +
	"\vsrc_node_id\x18\x02 \x01(\v2\x1b.api.colonio.v1alpha.NodeIDR\tsrcNodeId\x128\n" +
	"\x05offer\x18\x03 \x01(\v2 .api.colonio.v1alpha.SignalOfferH\x00R\x05offer\x12;\n" +
	"\x06answer\x18\x04 \x01(\v2!.api.colonio.v1alpha.SignalAnswerH\x00R\x06answer\x122\n" +
	"\x03ice\x18\x05 \x01(\v2\x1e.api.colonio.v1alpha.SignalICEH\x00R\x03iceB\t\n" +
	"\acontent\"t\n" +
	"\vSignalOffer\x12\x19\n" +
	"\boffer_id\x18\x01 \x01(\rR\aofferId\x128\n" +
	"\x04type\x18\x02 \x01(\x0e2$.api.colonio.v1alpha.SignalOfferTypeR\x04type\x12\x10\n" +
	"\x03sdp\x18\x03 \x01(\tR\x03sdp\"S\n" +
	"\fSignalAnswer\x12\x19\n" +
	"\boffer_id\x18\x01 \x01(\rR\aofferId\x12\x16\n" +
	"\x06status\x18\x02 \x01(\rR\x06status\x12\x10\n" +
	"\x03sdp\x18\x03 \x01(\tR\x03sdp\":\n" +
	"\tSignalICE\x12\x19\n" +
	"\boffer_id\x18\x01 \x01(\rR\aofferId\x12\x12\n" +
	"\x04ices\x18\x02 \x03(\tR\x04ices\"\x13\n" +
	"\x11AssignNodeRequest\"e\n" +
	"\x12AssignNodeResponse\x124\n" +
	"\anode_id\x18\x01 \x01(\v2\x1b.api.colonio.v1alpha.NodeIDR\x06nodeId\x12\x19\n" +
	"\bis_alone\x18\x02 \x01(\bR\aisAlone\"\x15\n" +
	"\x13UnassignNodeRequest\"\x16\n" +
	"\x14UnassignNodeResponse\"H\n" +
	"\x11SendSignalRequest\x123\n" +
	"\x06signal\x18\x01 \x01(\v2\x1b.api.colonio.v1alpha.SignalR\x06signal\"/\n" +
	"\x12SendSignalResponse\x12\x19\n" +
	"\bis_alone\x18\x01 \x01(\bR\aisAlone\"\x13\n" +
	"\x11PollSignalRequest\"K\n" +
	"\x12PollSignalResponse\x125\n" +
	"\asignals\x18\x01 \x03(\v2\x1b.api.colonio.v1alpha.SignalR\asignals*M\n" +
	"\x0fSignalOfferType\x12\x1e\n" +
	"\x1aSIGNAL_OFFER_TYPE_EXPLICIT\x10\x00\x12\x1a\n" +
	"\x16SIGNAL_OFFER_TYPE_NEXT\x10\x012\x99\x03\n" +
	"\vSeedService\x12_\n" +
	"\n" +
	"AssignNode\x12&.api.colonio.v1alpha.AssignNodeRequest\x1a'.api.colonio.v1alpha.AssignNodeResponse\"\x00\x12e\n" +
	"\fUnassignNode\x12(.api.colonio.v1alpha.UnassignNodeRequest\x1a).api.colonio.v1alpha.UnassignNodeResponse\"\x00\x12_\n" +
	"\n" +
	"SendSignal\x12&.api.colonio.v1alpha.SendSignalRequest\x1a'.api.colonio.v1alpha.SendSignalResponse\"\x00\x12a\n" +
	"\n" +
	"PollSignal\x12&.api.colonio.v1alpha.PollSignalRequest\x1a'.api.colonio.v1alpha.PollSignalResponse\"\x000\x01B5Z3github.com/llamerada-jp/colonio/api/colonio/v1alphab\x06proto3"

var (
	file_api_colonio_v1alpha_seed_proto_rawDescOnce sync.Once
	file_api_colonio_v1alpha_seed_proto_rawDescData []byte
)

func file_api_colonio_v1alpha_seed_proto_rawDescGZIP() []byte {
	file_api_colonio_v1alpha_seed_proto_rawDescOnce.Do(func() {
		file_api_colonio_v1alpha_seed_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_api_colonio_v1alpha_seed_proto_rawDesc), len(file_api_colonio_v1alpha_seed_proto_rawDesc)))
	})
	return file_api_colonio_v1alpha_seed_proto_rawDescData
}

var file_api_colonio_v1alpha_seed_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_api_colonio_v1alpha_seed_proto_msgTypes = make([]protoimpl.MessageInfo, 12)
var file_api_colonio_v1alpha_seed_proto_goTypes = []any{
	(SignalOfferType)(0),         // 0: api.colonio.v1alpha.SignalOfferType
	(*Signal)(nil),               // 1: api.colonio.v1alpha.Signal
	(*SignalOffer)(nil),          // 2: api.colonio.v1alpha.SignalOffer
	(*SignalAnswer)(nil),         // 3: api.colonio.v1alpha.SignalAnswer
	(*SignalICE)(nil),            // 4: api.colonio.v1alpha.SignalICE
	(*AssignNodeRequest)(nil),    // 5: api.colonio.v1alpha.AssignNodeRequest
	(*AssignNodeResponse)(nil),   // 6: api.colonio.v1alpha.AssignNodeResponse
	(*UnassignNodeRequest)(nil),  // 7: api.colonio.v1alpha.UnassignNodeRequest
	(*UnassignNodeResponse)(nil), // 8: api.colonio.v1alpha.UnassignNodeResponse
	(*SendSignalRequest)(nil),    // 9: api.colonio.v1alpha.SendSignalRequest
	(*SendSignalResponse)(nil),   // 10: api.colonio.v1alpha.SendSignalResponse
	(*PollSignalRequest)(nil),    // 11: api.colonio.v1alpha.PollSignalRequest
	(*PollSignalResponse)(nil),   // 12: api.colonio.v1alpha.PollSignalResponse
	(*NodeID)(nil),               // 13: api.colonio.v1alpha.NodeID
}
var file_api_colonio_v1alpha_seed_proto_depIdxs = []int32{
	13, // 0: api.colonio.v1alpha.Signal.dst_node_id:type_name -> api.colonio.v1alpha.NodeID
	13, // 1: api.colonio.v1alpha.Signal.src_node_id:type_name -> api.colonio.v1alpha.NodeID
	2,  // 2: api.colonio.v1alpha.Signal.offer:type_name -> api.colonio.v1alpha.SignalOffer
	3,  // 3: api.colonio.v1alpha.Signal.answer:type_name -> api.colonio.v1alpha.SignalAnswer
	4,  // 4: api.colonio.v1alpha.Signal.ice:type_name -> api.colonio.v1alpha.SignalICE
	0,  // 5: api.colonio.v1alpha.SignalOffer.type:type_name -> api.colonio.v1alpha.SignalOfferType
	13, // 6: api.colonio.v1alpha.AssignNodeResponse.node_id:type_name -> api.colonio.v1alpha.NodeID
	1,  // 7: api.colonio.v1alpha.SendSignalRequest.signal:type_name -> api.colonio.v1alpha.Signal
	1,  // 8: api.colonio.v1alpha.PollSignalResponse.signals:type_name -> api.colonio.v1alpha.Signal
	5,  // 9: api.colonio.v1alpha.SeedService.AssignNode:input_type -> api.colonio.v1alpha.AssignNodeRequest
	7,  // 10: api.colonio.v1alpha.SeedService.UnassignNode:input_type -> api.colonio.v1alpha.UnassignNodeRequest
	9,  // 11: api.colonio.v1alpha.SeedService.SendSignal:input_type -> api.colonio.v1alpha.SendSignalRequest
	11, // 12: api.colonio.v1alpha.SeedService.PollSignal:input_type -> api.colonio.v1alpha.PollSignalRequest
	6,  // 13: api.colonio.v1alpha.SeedService.AssignNode:output_type -> api.colonio.v1alpha.AssignNodeResponse
	8,  // 14: api.colonio.v1alpha.SeedService.UnassignNode:output_type -> api.colonio.v1alpha.UnassignNodeResponse
	10, // 15: api.colonio.v1alpha.SeedService.SendSignal:output_type -> api.colonio.v1alpha.SendSignalResponse
	12, // 16: api.colonio.v1alpha.SeedService.PollSignal:output_type -> api.colonio.v1alpha.PollSignalResponse
	13, // [13:17] is the sub-list for method output_type
	9,  // [9:13] is the sub-list for method input_type
	9,  // [9:9] is the sub-list for extension type_name
	9,  // [9:9] is the sub-list for extension extendee
	0,  // [0:9] is the sub-list for field type_name
}

func init() { file_api_colonio_v1alpha_seed_proto_init() }
func file_api_colonio_v1alpha_seed_proto_init() {
	if File_api_colonio_v1alpha_seed_proto != nil {
		return
	}
	file_api_colonio_v1alpha_colonio_proto_init()
	file_api_colonio_v1alpha_seed_proto_msgTypes[0].OneofWrappers = []any{
		(*Signal_Offer)(nil),
		(*Signal_Answer)(nil),
		(*Signal_Ice)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_api_colonio_v1alpha_seed_proto_rawDesc), len(file_api_colonio_v1alpha_seed_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   12,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_api_colonio_v1alpha_seed_proto_goTypes,
		DependencyIndexes: file_api_colonio_v1alpha_seed_proto_depIdxs,
		EnumInfos:         file_api_colonio_v1alpha_seed_proto_enumTypes,
		MessageInfos:      file_api_colonio_v1alpha_seed_proto_msgTypes,
	}.Build()
	File_api_colonio_v1alpha_seed_proto = out.File
	file_api_colonio_v1alpha_seed_proto_goTypes = nil
	file_api_colonio_v1alpha_seed_proto_depIdxs = nil
}
