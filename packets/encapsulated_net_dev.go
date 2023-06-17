package packets

import "io"

type EncapsulatedNetDevPacket []byte

func NewEncapsulateNetDevPacket(buf []byte) EncapsulatedNetDevPacket {
	return EncapsulatedNetDevPacket(buf)
}

func (*EncapsulatedNetDevPacket) dedicatedFunctionForMolePackets() {}
func (e *EncapsulatedNetDevPacket) WriteTo(buf []byte) error {
	if uint16(len(buf)) < e.TotalLength() {
		return ErrTooShort
	}
	copy(buf, []byte(*e))
	return nil
}
func (e *EncapsulatedNetDevPacket) FromBytes([]byte) error {
	return ErrNotImplemented
}
func (e *EncapsulatedNetDevPacket) TotalLength() uint16       { return uint16(len([]byte(*e))) }
func (e *EncapsulatedNetDevPacket) GetPacketType() PacketType { return EncapsulatedNetDevPacketType }
func (e *EncapsulatedNetDevPacket) WriteContentTo(w io.Writer) (int, error) {
	return w.Write([]byte(*e))
}
