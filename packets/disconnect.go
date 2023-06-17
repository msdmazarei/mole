package packets

type DisconnectCode byte

const (
	DisconnectTypeUnknwon DisconnectCode = iota
	DisconnectTypeNormal
	DisconnectTypeError
)

type DisconnectRequestPacket = StatusMessagePacket[DisconnectCode]
type DisconnectAcceptedPacket = StatusMessagePacket[DisconnectCode]

func NewDisconnectRequestPacket(msg string) DisconnectRequestPacket {
	return NewStatusMessage[DisconnectCode](DisconnectRequestType, 0, msg)
}

func NewDisconnectAcceptedPacket(msg string) DisconnectAcceptedPacket {
	return NewStatusMessage[DisconnectCode](DisconnectAcceptType, 0, msg)
}
