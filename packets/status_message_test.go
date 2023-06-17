package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StatusMessage", func() {
	const (
		sampleStatus  = 10
		sampleMessage = "hello"
	)
	var pkt StatusMessagePacket[byte]

	BeforeEach(func() {
		pkt = NewStatusMessage[byte](AuthRejectType, sampleStatus, sampleMessage)
	})
	It("Type Should Be AuthRejectPacket", func() {
		Expect(pkt.GetPacketType()).To(Equal(AuthRejectType))
	})
	It("Length Specifiers should carry correct values", func() {
		Expect(int(pkt.msgLen)).To(Equal(len(sampleMessage)))
		Expect(pkt.TotalLength()).To(Equal(2 + uint16(len(sampleMessage))))
	})

	It("Should Serialize correctly", func() {
		bys := make([]byte, pkt.TotalLength())
		Expect(pkt.WriteTo(bys)).To(BeNil())
		Expect(bys[0]).To(Equal(pkt.Status))
		Expect(bys[1]).To(Equal(byte(len(sampleMessage))))
		Expect(string(bys[2 : 2+pkt.msgLen])).To(Equal(pkt.Message))
	})

	It("From Bytes Should return error on invalid input", func() {
		a := AuthRejectPacket{}
		Expect(a.FromBytes([]byte{})).Error().Should(Equal(ErrTooShort))
		Expect(a.FromBytes([]byte{1, 2, 3})).Error().Should(Equal(ErrTooShort))
	})

	It("FromBytes Should Return Expected Result", func() {
		a := AuthRejectPacket{}
		Expect(a.FromBytes([]byte{1, 1, 'A'})).To(BeNil())
		Expect(a.msgLen).To(Equal(byte(1)))
		Expect(a.Status).To(Equal(RejectCode(1)))
		Expect(a.Message).To(Equal("A"))
	})

})
