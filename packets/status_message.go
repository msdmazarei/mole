package packets

type StatusMessagePacket[S ~byte] struct {
	Status     S
	msgLen     byte
	Message    string
	packetType PacketType
}

func NewStatusMessage[S ~byte](pkt_type PacketType, code S, msg string) StatusMessagePacket[S] {
	return StatusMessagePacket[S]{
		Status:     code,
		msgLen:     byte(len(msg)),
		Message:    msg,
		packetType: pkt_type,
	}
}
func (*StatusMessagePacket[S]) dedicatedFunctionForMolePackets() {}
func (a *StatusMessagePacket[S]) WriteTo(buf []byte) error {
	if uint16(len(buf)) < a.TotalLength() {
		return ErrTooShort
	}
	buf[0] = byte(a.Status)
	buf[1] = a.msgLen
	copy(buf[2:], []byte(a.Message))
	return nil
}
func (a *StatusMessagePacket[S]) FromBytes(buf []byte) error {
	l := len(buf)
	if l < 2 {
		return ErrTooShort
	}
	if l < 2+int(buf[1]) {
		return ErrTooShort
	}
	a.Status = S(buf[0])
	a.msgLen = buf[1]
	a.Message = string(buf[2 : 2+buf[1]])
	return nil
}
func (a *StatusMessagePacket[S]) TotalLength() uint16 {
	return 2 + uint16(a.msgLen)
}

func (a *StatusMessagePacket[S]) GetPacketType() PacketType {
	return a.packetType
}
