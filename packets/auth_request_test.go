package packets

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	username    = "username"
	secret      = "secret"
	wrongSecret = "wrongSecret"
)

var _ = Describe("AuthRequest", func() {
	var authPacket AuthRequestPacket
	BeforeEach(func() {
		authPacket = NewAuhRequestPacket(username, secret)
	})
	It("Its Type Should Be AuthRequestType", func() {
		Expect(authPacket.GetPacketType()).To(Equal(AuthRequestType))
	})
	It("Length Specifiers should carry correct values", func() {
		Expect(int(authPacket.hashedStringLength)).To(Equal(len(authPacket.HashedString)))
		Expect(int(authPacket.randomStringLength)).To(Equal(len(authPacket.RandomString)))
		Expect(int(authPacket.usernameLength)).To(Equal(len(authPacket.Username)))
		Expect(authPacket.TotalLength()).To(
			Equal(3 +
				uint16(authPacket.hashedStringLength) +
				uint16(authPacket.randomStringLength) +
				uint16(authPacket.usernameLength)),
		)
	})

	It("Authenticate Properly using passed secret", func() {
		Expect(authPacket.Authenticate(wrongSecret)).To(Equal(false))
		Expect(authPacket.Authenticate(secret)).To(Equal(true))
	})

	It("Should Serialize correctly", func() {
		bys := make([]byte, authPacket.TotalLength())
		Expect(authPacket.WriteTo(bys)).To(BeNil())
		Expect(bys[0]).To(Equal(authPacket.randomStringLength))
		Expect(bys[1]).To(Equal(authPacket.hashedStringLength))
		Expect(bys[2]).To(Equal(authPacket.usernameLength))
		i := 3
		Expect(string(bys[i : i+int(authPacket.randomStringLength)])).To(Equal(authPacket.RandomString))
		i += int(authPacket.randomStringLength)
		Expect(string(bys[i : i+int(authPacket.hashedStringLength)])).To(Equal(authPacket.HashedString))
		i += int(authPacket.hashedStringLength)
		Expect(string(bys[i : i+int(authPacket.usernameLength)])).To(Equal(authPacket.Username))
	})

	It("From Bytes Should return error on invalid input", func() {
		a := AuthRequestPacket{}
		Expect(a.FromBytes([]byte{})).Error().Should(Equal(ErrTooShort))
		Expect(a.FromBytes([]byte{1, 2, 3})).Error().Should(Equal(ErrTooShort))
		Expect(a.FromBytes([]byte{5, 12, 1, 2, 3})).Error().Should(Equal(ErrTooShort))
	})

	It("FromBytes Should Return Expected Result", func() {
		a := AuthRequestPacket{}
		Expect(a.FromBytes([]byte{1, 1, 1, 'A', 'B', 'C'})).To(BeNil())
		Expect(a.hashedStringLength).To(Equal(byte(1)))
		Expect(a.randomStringLength).To(Equal(byte(1)))
		Expect(a.usernameLength).To(Equal(byte(1)))
		Expect(a.RandomString).To(Equal("A"))
		Expect(a.HashedString).To(Equal("B"))
		Expect(a.Username).To(Equal("C"))

	})

})
