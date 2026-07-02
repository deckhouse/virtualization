//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package expectations

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExpectations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualMachinePool Expectations Suite")
}
