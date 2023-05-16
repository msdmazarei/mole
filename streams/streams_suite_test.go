package streams_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStreams(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streams Suite")
}
