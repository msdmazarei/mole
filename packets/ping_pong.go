package packets
type PingCode byte
const (
	PingCodeUnknown  = iota
	PingCodeNormal
	PingCodeUrgent
)
type PingPacket = StatusMessagePacket[PingCode]
type PongPacket = StatusMessagePacket[PingCode]

func NewPingPacket(msg string) PingPacket {
	return NewStatusMessage[PingCode](PingType, 0, msg)
}

func NewPongPacket(msg string) PongPacket {
	return NewStatusMessage[PingCode](PongType, 0, msg)
}

