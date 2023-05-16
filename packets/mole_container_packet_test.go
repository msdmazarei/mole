package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MoleContainerPacket", func() {
	const sample_message = "sample_message"
	const sample_status = 100
	It("Type Should Be MoleContainerType", func() {
		pkt := NewMoleContainerPacket(nil)
		Expect(pkt.GetPacketType()).To(Equal(ContainerPacketType))
	})
	It("Length Specifiers should carry correct values", func() {
		pkt := NewMoleContainerPacket(nil)
		Expect(pkt.TotalLength()).To(Equal(uint16(3)))
		ping_pkt := NewPingPacket(sample_message)
		pkt = NewMoleContainerPacket(&ping_pkt)
		Expect(pkt.TotalLength()).To(Equal(uint16(3 + ping_pkt.TotalLength())))
	})

	It("Should Serialize StatusMessages Correctly", func() {
		auth_acc_pkt := NewAuthAcceptPacket(sample_status, sample_message)
		pkt := NewMoleContainerPacket(&auth_acc_pkt)
		pkt_buf := make([]byte, pkt.TotalLength())
		auth_acc_buf := make([]byte, auth_acc_pkt.TotalLength())
		Expect(auth_acc_pkt.WriteTo(auth_acc_buf)).To(BeNil())
		Expect(pkt.WriteTo(pkt_buf)).To(BeNil())
		Expect(pkt_buf[3:]).To(Equal(auth_acc_buf))
		Expect(pkt_buf[2]).To(Equal(byte(auth_acc_pkt.GetPacketType())))
	})
	It("Should Serialize EncapsulateNetDevPacket Correctly", func() {
		child_pkt := NewEncapsulateNetDevPacket([]byte{1, 2, 3, 4, 5, 6, 7})
		pkt := NewMoleContainerPacket(&child_pkt)
		pkt_buf := make([]byte, pkt.TotalLength())
		child_buf := make([]byte, child_pkt.TotalLength())
		Expect(child_pkt.WriteTo(child_buf)).To(BeNil())
		Expect(pkt.WriteTo(pkt_buf)).To(BeNil())
		Expect(pkt_buf[3:]).To(Equal(child_buf))
		Expect(pkt_buf[2]).To(Equal(byte(child_pkt.GetPacketType())))

	})
	It("Should Serialize ReportPacket Correctly", func() {
		child_pkt := NewReportPacket(1, 2, 3, 4, 5)
		pkt := NewMoleContainerPacket(&child_pkt)
		pkt_buf := make([]byte, pkt.TotalLength())
		child_buf := make([]byte, child_pkt.TotalLength())
		Expect(child_pkt.WriteTo(child_buf)).To(BeNil())
		Expect(pkt.WriteTo(pkt_buf)).To(BeNil())
		Expect(pkt_buf[3:]).To(Equal(child_buf))
		Expect(pkt_buf[2]).To(Equal(byte(child_pkt.GetPacketType())))
	})

	It("From Bytes Should return error on invalid input", func() {
		pkt := MoleContainerPacket{}
		Expect(pkt.FromBytes([]byte{10, 1, 2, 3})).Error().To(Equal(ErrBadPacketFormat))
		Expect(pkt.FromBytes([]byte{0, 1, 2, 3})).Error().To(Equal(ErrBadPacketFormat))
		Expect(pkt.FromBytes([]byte{0, 5, 1, 2, 3, 4})).Error().To(Equal(ErrTooShort))
		Expect(pkt.FromBytes([]byte{0, 0, 0, 0, 0, 0, 0})).Error().To(Equal(ErrBadPacketFormat))

	})

	It("FromBytes Should Return Expected Result For StatusMessage", func() {
		const sample_code = 100
		const sample_msg = "sample_msg"
		child_pkt := NewAuthAcceptPacket(sample_code, sample_msg)
		pkt := NewMoleContainerPacket(&child_pkt)
		pkt_buf := make([]byte, pkt.TotalLength())
		pkt.WriteTo(pkt_buf)

		deser_pkt := MoleContainerPacket{}
		Expect(deser_pkt.FromBytes(pkt_buf)).To(BeNil())
		Expect(deser_pkt.PacketType).To(Equal(child_pkt.GetPacketType()))
		Expect(deser_pkt.MolePacket.TotalLength()).To(Equal(child_pkt.TotalLength()))
		Expect(deser_pkt.MolePacket.(*AuthAcceptPacket).Status).To(Equal(child_pkt.Status))
		Expect(deser_pkt.MolePacket.(*AuthAcceptPacket).Message).To(Equal(child_pkt.Message))
	})

	It("FromBytes Should Return Expected Result For EncapsulatedNetDevPacket", func() {
		captured_buf := []byte{1, 2, 3, 4, 5, 6, 7}
		child_pkt := NewEncapsulateNetDevPacket(captured_buf)
		pkt := NewMoleContainerPacket(&child_pkt)
		pkt_buf := make([]byte, pkt.TotalLength())
		pkt.WriteTo(pkt_buf)

		deser_pkt := MoleContainerPacket{}
		Expect(deser_pkt.FromBytes(pkt_buf)).To(BeNil())
		Expect(deser_pkt.PacketType).To(Equal(EncapsulatedNetDevPacketType))
		Expect(deser_pkt.MolePacket.TotalLength()).To(Equal(uint16(len(captured_buf))))

	})

	It("FromBytes Should Return Expected Result For ReportPacket", func() {

		child_pkt := NewReportPacket(1, 2, 3, 4, 5)
		pkt := NewMoleContainerPacket(&child_pkt)
		pkt_buf := make([]byte, pkt.TotalLength())
		pkt.WriteTo(pkt_buf)

		deser_pkt := MoleContainerPacket{}
		Expect(deser_pkt.FromBytes(pkt_buf)).To(BeNil())
		Expect(deser_pkt.PacketType).To(Equal(child_pkt.GetPacketType()))
		Expect(deser_pkt.MolePacket.TotalLength()).To(Equal(child_pkt.TotalLength()))
		Expect(deser_pkt.MolePacket.(*ReportPacket).SystemTime).To(Equal(uint64(1)))
		Expect(deser_pkt.MolePacket.(*ReportPacket).SentPackets).To(Equal(uint64(2)))
		Expect(deser_pkt.MolePacket.(*ReportPacket).RecvPackets).To(Equal(uint64(3)))
	})

})
