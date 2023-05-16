package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	secret       = "secret"
	wrong_secret = "wrong_secret"
)

var _ = Describe("AuthRequest", func() {
	var auth_packet AuthRequestPacket
	BeforeEach(func() {
		auth_packet = NewAuhRequestPacket(secret)
	})
	It("Its Type Should Be AuthRequestType", func() {
		Expect(auth_packet.GetPacketType()).To(Equal(AuthRequestType))
	})
	It("Length Specifiers should carry correct values", func() {
		Expect(int(auth_packet.hashedStringLength)).To(Equal(len(auth_packet.HashedString)))
		Expect(int(auth_packet.randomStringLength)).To(Equal(len(auth_packet.RandomString)))
		Expect(auth_packet.TotalLength()).To(Equal(2 + uint16(auth_packet.hashedStringLength) + uint16(auth_packet.randomStringLength)))
	})

	It("Authenticate Properly using passed secret", func() {
		Expect(auth_packet.Authenticate(wrong_secret)).To(Equal(false))
		Expect(auth_packet.Authenticate(secret)).To(Equal(true))
	})

	It("Should Serialize correctly", func() {
		bys := make([]byte, auth_packet.TotalLength())
		Expect(auth_packet.WriteTo(bys)).To(BeNil())
		Expect(bys[0]).To(Equal(auth_packet.randomStringLength))
		Expect(bys[1]).To(Equal(auth_packet.hashedStringLength))
		Expect(string(bys[2 : 2+auth_packet.randomStringLength])).To(Equal(auth_packet.RandomString))
		Expect(string(bys[2+auth_packet.randomStringLength:])).To(Equal(auth_packet.HashedString))
	})

	It("From Bytes Should return error on invalid input", func() {
		a := AuthRequestPacket{}
		Expect(a.FromBytes([]byte{})).Error().Should(Equal(ErrTooShort))
		Expect(a.FromBytes([]byte{1, 2, 3})).Error().Should(Equal(ErrTooShort))
		Expect(a.FromBytes([]byte{5, 12, 1, 2, 3})).Error().Should(Equal(ErrTooShort))
	})

	It("FromBytes Should Return Expected Result", func() {
		a := AuthRequestPacket{}
		Expect(a.FromBytes([]byte{1, 1, 'A', 'B'})).To(BeNil())
		Expect(a.hashedStringLength).To(Equal(byte(1)))
		Expect(a.randomStringLength).To(Equal(byte(1)))
		Expect(a.RandomString).To(Equal("A"))
		Expect(a.HashedString).To(Equal("B"))

	})

})
