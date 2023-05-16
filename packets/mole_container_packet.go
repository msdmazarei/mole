package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MoleContainerPacket struct {
	PacketType PacketType
	length     uint16
	MolePacket MolePacketer
}

func NewMoleContainerPacket(molePacket MolePacketer) MoleContainerPacket {
	mole_packet_len := uint16(0)
	var pkt_type PacketType
	if molePacket != nil {
		mole_packet_len = molePacket.TotalLength()
		pkt_type = molePacket.GetPacketType()
	} else {
		pkt_type = ContainerPacketType
	}
	return MoleContainerPacket{
		length:     3 + mole_packet_len,
		PacketType: pkt_type,
		MolePacket: molePacket,
	}
}

func (*MoleContainerPacket) dedicatedFunctionForMolePackets() {}
func (mcp *MoleContainerPacket) WriteTo(buf []byte) error {
	if uint16(len(buf)) < mcp.TotalLength() {
		return ErrTooShort
	}
	binary.BigEndian.PutUint16(buf[0:], mcp.length)
	buf[2] = byte(mcp.MolePacket.GetPacketType())
	if mcp.MolePacket != nil {
		return mcp.MolePacket.WriteTo(buf[3:])
	}
	return nil

}
func (mcp *MoleContainerPacket) FromBytes(buf []byte) error {
	mcp.length = binary.BigEndian.Uint16(buf[0:])
	mcp.PacketType = PacketType(buf[2])
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
		encpkt := NewEncapsulateNetDevPacket(buf[3:mcp.length])
		mcp.MolePacket = &encpkt
	default:
		fmt.Printf("receiving not parsable packet: %+v\n", buf)
		return ErrNotImplemented
	}
	var err error
	if mcp.PacketType != EncapsulatedNetDevPacketType {
		err = mcp.MolePacket.FromBytes(buf[3:])
	}
	if mcp.length != mcp.MolePacket.TotalLength()+3 {
		err = ErrBadPacketFormat
	}
	return err
}

func (mcp *MoleContainerPacket) TotalLength() uint16 {
	if mcp.MolePacket == nil {
		return 3
	}
	return 3 + mcp.MolePacket.TotalLength()
}
func (mcp *MoleContainerPacket) GetPacketType() PacketType {
	return ContainerPacketType
}
func (mcp *MoleContainerPacket) Write(w io.Writer) (int, error) {
	buf := make([]byte, mcp.TotalLength())
	mcp.WriteTo(buf)
	return w.Write(buf)
}
