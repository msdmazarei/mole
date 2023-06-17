package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"math/rand"
)

var _ = Describe("EncapsulatedNetDevPacket", func() {
	var pkt EncapsulatedNetDevPacket
	BeforeEach(func() {
		bufl := rand.Intn(100) + 10
		buf := make([]byte, bufl)
		rand.Read(buf)
		pkt = NewEncapsulateNetDevPacket(buf)
	})
	It("Its Type Should Be AuthRequestType", func() {
		Expect(pkt.GetPacketType()).To(Equal(EncapsulatedNetDevPacketType))
	})
	It("Length Specifiers should carry correct values", func() {
		Expect(int(pkt.TotalLength())).To(Equal(len([]byte(pkt))))
	})

	It("Should Serialize correctly", func() {
		buf := make([]byte, pkt.TotalLength())
		Expect(pkt.WriteTo(buf)).To(BeNil())
		b := []byte(pkt)
		for i, bval := range b {
			Expect(buf[i]).To(Equal(bval))
		}
	})

	It("FromBytes Should Return Expected Result", func() {
		buf := []byte{1, 2, 3, 4, 5}
		e := pkt.FromBytes(buf)
		Expect(e).Error().To(Equal(ErrNotImplemented))
	})

})
