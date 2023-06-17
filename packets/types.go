package packets

import (
	"errors"
)

type PacketType byte

const (
	AuthRequestType PacketType = iota
	AuthRejectType
	AuthAcceptType

	PingType
	PongType

	ReportType

	EncapsulatedNetDevPacketType

	DisconnectRequestType
	DisconnectAcceptType

	ContainerPacketType
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
