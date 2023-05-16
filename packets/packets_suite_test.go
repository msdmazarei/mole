package packets

import (
	"testing"

	// "github.com/msdmazarei/mole/packets"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPackets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Packets Suite")
}


/*var _ = Describe("Mole Packet Tests", func() {
	Context("AuthRequest", func() {
		It("Check Total Length", func() {
			a := NewAuhRequestPacket(secret)
			Expect((&a).Authenticate(wrong_secret)).To(Equal(false))
		})

	})
})
*/
