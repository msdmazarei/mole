package packets

import "encoding/binary"

type ReportPacket struct {
	SystemTime  uint64
	SentPackets uint64
	RecvPackets uint64
	SentBytes   uint64
	RecvBytes   uint64
}

func NewReportPacket(system_time, sent_packets, recv_packets, sent_bytes, recv_bytes uint64) ReportPacket {
	return ReportPacket{
		SystemTime:  system_time,
		SentPackets: sent_packets,
		RecvPackets: recv_packets,
		SentBytes:   sent_bytes,
		RecvBytes:   recv_bytes,
	}
}
func (*ReportPacket) dedicatedFunctionForMolePackets() {}
func (r *ReportPacket) WriteTo(buf []byte) error {
	if len(buf) < int(r.TotalLength()) {
		return ErrTooShort
	}
	i := 0
	binary.BigEndian.PutUint64(buf[i:], r.SystemTime)
	i += 8
	binary.BigEndian.PutUint64(buf[i:], r.SentPackets)
	i += 8
	binary.BigEndian.PutUint64(buf[i:], r.RecvPackets)
	i += 8
	binary.BigEndian.PutUint64(buf[i:], r.SentBytes)
	i += 8
	binary.BigEndian.PutUint64(buf[i:], r.RecvBytes)
	i += 8
	return nil
}
func (r *ReportPacket) FromBytes(buf []byte) error {
	if len(buf) < int(r.TotalLength()) {
		return ErrTooShort
	}
	i := 0
	r.SystemTime = binary.BigEndian.Uint64(buf[i:])
	i += 8
	r.SentPackets = binary.BigEndian.Uint64(buf[i:])
	i += 8
	r.RecvPackets = binary.BigEndian.Uint64(buf[i:])
	i += 8
	r.SentBytes = binary.BigEndian.Uint64(buf[i:])
	i += 8
	r.RecvBytes = binary.BigEndian.Uint64(buf[i:])
	i += 8
	return nil
}
func (r *ReportPacket) TotalLength() uint16       { return 5 * 8 }
func (r *ReportPacket) GetPacketType() PacketType { return ReportType }
