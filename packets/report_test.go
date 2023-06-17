package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Report", func() {
	var pkt ReportPacket

	BeforeEach(func() {
		pkt = NewReportPacket(1, 2, 3, 4, 5)
	})
	It("Type Should Be ReportType", func() {
		Expect(pkt.GetPacketType()).To(Equal(ReportType))
	})
	It("Length Specifiers should carry correct values", func() {
		Expect(pkt.TotalLength()).To(Equal(uint16(40)))
	})

	It("Should Serialize correctly", func() {
		bys := make([]byte, pkt.TotalLength())
		Expect(pkt.WriteTo(bys)).To(BeNil())
		Expect(bys).To(Equal([]byte{
			0, 0, 0, 0, 0, 0, 0, 1,
			0, 0, 0, 0, 0, 0, 0, 2,
			0, 0, 0, 0, 0, 0, 0, 3,
			0, 0, 0, 0, 0, 0, 0, 4,
			0, 0, 0, 0, 0, 0, 0, 5,
		}))
	})

	It("From Bytes Should return error on invalid input", func() {

		Expect(pkt.FromBytes([]byte{})).Error().Should(Equal(ErrTooShort))
		Expect(pkt.FromBytes([]byte{1, 2, 3})).Error().Should(Equal(ErrTooShort))
	})

	It("FromBytes Should Return Expected Result", func() {
		Expect(pkt.FromBytes(
			[]byte{
				0, 0, 0, 0, 0, 0, 0, 7,
				0, 0, 0, 0, 0, 0, 0, 8,
				0, 0, 0, 0, 0, 0, 0, 9,
				0, 0, 0, 0, 0, 0, 0, 10,
				0, 0, 0, 0, 0, 0, 0, 11,
			})).To(BeNil())
		Expect(pkt.SystemTime).To(Equal(uint64(7)))
		Expect(pkt.SentPackets).To(Equal(uint64(8)))
		Expect(pkt.RecvPackets).To(Equal(uint64(9)))
		Expect(pkt.SentBytes).To(Equal(uint64(10)))
		Expect(pkt.RecvBytes).To(Equal(uint64(11)))

	})

})
