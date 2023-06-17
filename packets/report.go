package packets

import "encoding/binary"

type ReportPacket struct {
	SystemTime  uint64
	SentPackets uint64
	RecvPackets uint64
	SentBytes   uint64
	RecvBytes   uint64
}

const (
	sizeOfUint64      = 8
	reportFieldsCount = 5
)

func NewReportPacket(systemTime, sentPackets, recvPackets, sentBytes, recvBytes uint64) ReportPacket {
	return ReportPacket{
		SystemTime:  systemTime,
		SentPackets: sentPackets,
		RecvPackets: recvPackets,
		SentBytes:   sentBytes,
		RecvBytes:   recvBytes,
	}
}
func (*ReportPacket) dedicatedFunctionForMolePackets() {}
func (r *ReportPacket) WriteTo(buf []byte) error {
	if len(buf) < int(r.TotalLength()) {
		return ErrTooShort
	}
	i := 0
	binary.BigEndian.PutUint64(buf[i:], r.SystemTime)
	i += sizeOfUint64
	binary.BigEndian.PutUint64(buf[i:], r.SentPackets)
	i += sizeOfUint64
	binary.BigEndian.PutUint64(buf[i:], r.RecvPackets)
	i += sizeOfUint64
	binary.BigEndian.PutUint64(buf[i:], r.SentBytes)
	i += sizeOfUint64
	binary.BigEndian.PutUint64(buf[i:], r.RecvBytes)
	return nil
}
func (r *ReportPacket) FromBytes(buf []byte) error {
	if len(buf) < int(r.TotalLength()) {
		return ErrTooShort
	}
	i := 0
	r.SystemTime = binary.BigEndian.Uint64(buf[i:])
	i += sizeOfUint64
	r.SentPackets = binary.BigEndian.Uint64(buf[i:])
	i += sizeOfUint64
	r.RecvPackets = binary.BigEndian.Uint64(buf[i:])
	i += sizeOfUint64
	r.SentBytes = binary.BigEndian.Uint64(buf[i:])
	i += sizeOfUint64
	r.RecvBytes = binary.BigEndian.Uint64(buf[i:])
	return nil
}
func (r *ReportPacket) TotalLength() uint16       { return reportFieldsCount * sizeOfUint64 }
func (r *ReportPacket) GetPacketType() PacketType { return ReportType }
