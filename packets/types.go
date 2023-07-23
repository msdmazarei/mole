package packets

import (
	"errors"
)

type PacketType byte

const (
	AuthRequestType PacketType = 0
	AuthRejectType  PacketType = 1
	AuthAcceptType  PacketType = 2

	PingType PacketType = 3
	PongType PacketType = 4

	ReportType PacketType = 5

	EncapsulatedNetDevPacketType PacketType = 6

	DisconnectRequestType PacketType = 7
	DisconnectAcceptType  PacketType = 8

	ContainerPacketType PacketType = 9
)

var (
	ErrTooShort        = errors.New("packets.buffer: too short buffer")
	ErrNotImplemented  = errors.New("method Is not implemented")
	ErrBadPacketFormat = errors.New("bad packet format")
)

type MolePacketer interface {
	dedicatedFunctionForMolePackets()
	WriteTo([]byte) error
	FromBytes([]byte) error
	TotalLength() uint16
	GetPacketType() PacketType
}
