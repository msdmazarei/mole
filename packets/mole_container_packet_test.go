package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MoleContainerPacket", func() {
	const sampleMessage = "sampleMessage"
	const sampleStatus = 100
	It("Type Should Be MoleContainerType", func() {
		pkt := NewMoleContainerPacket(nil)
		Expect(pkt.GetPacketType()).To(Equal(ContainerPacketType))
	})
	It("Length Specifiers should carry correct values", func() {
		pkt := NewMoleContainerPacket(nil)
		Expect(pkt.TotalLength()).To(Equal(uint16(3)))
		pingPkt := NewPingPacket(sampleMessage)
		pkt = NewMoleContainerPacket(&pingPkt)
		Expect(pkt.TotalLength()).To(Equal(3 + pingPkt.TotalLength()))
	})

	It("Should Serialize StatusMessages Correctly", func() {
		authAccPkt := NewAuthAcceptPacket(sampleStatus, sampleMessage)
		pkt := NewMoleContainerPacket(&authAccPkt)
		pktBuf := make([]byte, pkt.TotalLength())
		authAccBuf := make([]byte, authAccPkt.TotalLength())
		Expect(authAccPkt.WriteTo(authAccBuf)).To(BeNil())
		Expect(pkt.WriteTo(pktBuf)).To(BeNil())
		Expect(pktBuf[3:]).To(Equal(authAccBuf))
		Expect(pktBuf[2]).To(Equal(byte(authAccPkt.GetPacketType())))
	})
	It("Should Serialize EncapsulateNetDevPacket Correctly", func() {
		childPkt := NewEncapsulateNetDevPacket([]byte{1, 2, 3, 4, 5, 6, 7})
		pkt := NewMoleContainerPacket(&childPkt)
		pktBuf := make([]byte, pkt.TotalLength())
		childBuf := make([]byte, childPkt.TotalLength())
		Expect(childPkt.WriteTo(childBuf)).To(BeNil())
		Expect(pkt.WriteTo(pktBuf)).To(BeNil())
		Expect(pktBuf[3:]).To(Equal(childBuf))
		Expect(pktBuf[2]).To(Equal(byte(childPkt.GetPacketType())))

	})
	It("Should Serialize ReportPacket Correctly", func() {
		childPkt := NewReportPacket(1, 2, 3, 4, 5)
		pkt := NewMoleContainerPacket(&childPkt)
		pktBuf := make([]byte, pkt.TotalLength())
		childBuf := make([]byte, childPkt.TotalLength())
		Expect(childPkt.WriteTo(childBuf)).To(BeNil())
		Expect(pkt.WriteTo(pktBuf)).To(BeNil())
		Expect(pktBuf[3:]).To(Equal(childBuf))
		Expect(pktBuf[2]).To(Equal(byte(childPkt.GetPacketType())))
	})

	It("From Bytes Should return error on invalid input", func() {
		pkt := MoleContainerPacket{}
		Expect(pkt.FromBytes([]byte{10, 1, 2, 3})).Error().To(Equal(ErrBadPacketFormat))
		Expect(pkt.FromBytes([]byte{0, 1, 2, 3})).Error().To(Equal(ErrBadPacketFormat))
		Expect(pkt.FromBytes([]byte{0, 5, 1, 2, 3, 4})).Error().To(Equal(ErrTooShort))
		Expect(pkt.FromBytes([]byte{0, 0, 0, 0, 0, 0, 0})).Error().To(Equal(ErrBadPacketFormat))

	})

	It("FromBytes Should Return Expected Result For StatusMessage", func() {
		const sampleCode = 100
		const sampleMsg = "sampleMsg"
		childPkt := NewAuthAcceptPacket(sampleCode, sampleMsg)
		pkt := NewMoleContainerPacket(&childPkt)
		pktBuf := make([]byte, pkt.TotalLength())
		Expect(pkt.WriteTo(pktBuf)).To(BeNil())

		deserPkt := MoleContainerPacket{}
		Expect(deserPkt.FromBytes(pktBuf)).To(BeNil())
		Expect(deserPkt.PacketType).To(Equal(childPkt.GetPacketType()))
		Expect(deserPkt.MolePacket.TotalLength()).To(Equal(childPkt.TotalLength()))
		Expect(deserPkt.MolePacket.(*AuthAcceptPacket).Status).To(Equal(childPkt.Status))
		Expect(deserPkt.MolePacket.(*AuthAcceptPacket).Message).To(Equal(childPkt.Message))
	})

	It("FromBytes Should Return Expected Result For EncapsulatedNetDevPacket", func() {
		capturedBuf := []byte{1, 2, 3, 4, 5, 6, 7}
		childPkt := NewEncapsulateNetDevPacket(capturedBuf)
		pkt := NewMoleContainerPacket(&childPkt)
		pktBuf := make([]byte, pkt.TotalLength())
		Expect(pkt.WriteTo(pktBuf)).To(BeNil())

		deserPkt := MoleContainerPacket{}
		Expect(deserPkt.FromBytes(pktBuf)).To(BeNil())
		Expect(deserPkt.PacketType).To(Equal(EncapsulatedNetDevPacketType))
		Expect(deserPkt.MolePacket.TotalLength()).To(Equal(uint16(len(capturedBuf))))

	})

	It("FromBytes Should Return Expected Result For ReportPacket", func() {

		childPkt := NewReportPacket(1, 2, 3, 4, 5)
		pkt := NewMoleContainerPacket(&childPkt)
		pktBuf := make([]byte, pkt.TotalLength())
		Expect(pkt.WriteTo(pktBuf)).To(BeNil())

		deserPkt := MoleContainerPacket{}
		Expect(deserPkt.FromBytes(pktBuf)).To(BeNil())
		Expect(deserPkt.PacketType).To(Equal(childPkt.GetPacketType()))
		Expect(deserPkt.MolePacket.TotalLength()).To(Equal(childPkt.TotalLength()))
		Expect(deserPkt.MolePacket.(*ReportPacket).SystemTime).To(Equal(uint64(1)))
		Expect(deserPkt.MolePacket.(*ReportPacket).SentPackets).To(Equal(uint64(2)))
		Expect(deserPkt.MolePacket.(*ReportPacket).RecvPackets).To(Equal(uint64(3)))
	})

})
