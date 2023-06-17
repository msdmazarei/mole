package packets

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPackets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Packets Suite")
}
