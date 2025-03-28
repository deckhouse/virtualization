package vm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVirtualizationArtifact(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Events Test Suite")
}
