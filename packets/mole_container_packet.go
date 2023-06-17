package packets

import (
	"encoding/binary"
	"io"

	"github.com/sirupsen/logrus"
)

type MoleContainerPacket struct {
	PacketType PacketType
	length     uint16
	MolePacket MolePacketer
}

const (
	containerPacketHeaderLen = 3
	indexOfPktType = 2
)

func NewMoleContainerPacket(molePacket MolePacketer) MoleContainerPacket {
	molePacketLen := uint16(0)
	var pktType PacketType
	if molePacket != nil {
		molePacketLen = molePacket.TotalLength()
		pktType = molePacket.GetPacketType()
	} else {
		pktType = ContainerPacketType
	}
	return MoleContainerPacket{
		length:     containerPacketHeaderLen + molePacketLen,
		PacketType: pktType,
		MolePacket: molePacket,
	}
}

func (*MoleContainerPacket) dedicatedFunctionForMolePackets() {}
func (mcp *MoleContainerPacket) WriteTo(buf []byte) error {
	if uint16(len(buf)) < mcp.TotalLength() {
		return ErrTooShort
	}
	binary.BigEndian.PutUint16(buf, mcp.length)
	buf[indexOfPktType] = byte(mcp.MolePacket.GetPacketType())
	if mcp.MolePacket != nil {
		return mcp.MolePacket.WriteTo(buf[containerPacketHeaderLen:])
	}
	return nil
}
func (mcp *MoleContainerPacket) FromBytes(buf []byte) error {
	mcp.length = binary.BigEndian.Uint16(buf)
	mcp.PacketType = PacketType(buf[indexOfPktType])
	switch mcp.PacketType {
	case AuthAcceptType:
		mcp.MolePacket = &AuthAcceptPacket{}
	case AuthRejectType:
		mcp.MolePacket = &AuthRejectPacket{}
	case AuthRequestType:
		mcp.MolePacket = &AuthRequestPacket{}
	case PingType:
		mcp.MolePacket = &PingPacket{}
	case PongType:
		mcp.MolePacket = &PongPacket{}
	case DisconnectRequestType:
		mcp.MolePacket = &DisconnectRequestPacket{}
	case DisconnectAcceptType:
		mcp.MolePacket = &DisconnectAcceptedPacket{}
	case ReportType:
		mcp.MolePacket = &ReportPacket{}
	case EncapsulatedNetDevPacketType:
		encpkt := NewEncapsulateNetDevPacket(buf[containerPacketHeaderLen:mcp.length])
		mcp.MolePacket = &encpkt
	default:
		logrus.Warn("receiving not parsable packet:", buf)
		return ErrNotImplemented
	}
	var err error
	if mcp.PacketType != EncapsulatedNetDevPacketType {
		err = mcp.MolePacket.FromBytes(buf[containerPacketHeaderLen:])
	}
	if mcp.length != mcp.MolePacket.TotalLength()+containerPacketHeaderLen {
		err = ErrBadPacketFormat
	}
	return err
}

func (mcp *MoleContainerPacket) TotalLength() uint16 {
	if mcp.MolePacket == nil {
		return containerPacketHeaderLen
	}
	return containerPacketHeaderLen + mcp.MolePacket.TotalLength()
}
func (mcp *MoleContainerPacket) GetPacketType() PacketType {
	return ContainerPacketType
}
func (mcp *MoleContainerPacket) Write(w io.Writer) (int, error) {
	buf := make([]byte, mcp.TotalLength())
	err := mcp.WriteTo(buf)
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}
