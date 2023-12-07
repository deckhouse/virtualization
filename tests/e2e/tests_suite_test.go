package e2e_test

import (
	"fmt"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

const (
	testNamespace          = "test-d8-virtualization"
	ShortWaitDuration      = 60 * time.Second
	LongWaitDuration       = 300 * time.Second
	PhaseReady             = "Ready"
	PhaseSucceeded         = "Succeeded"
	PhaseWaitForUserUpload = "WaitForUserUpload"
	TestdataDir            = "./testdata"
)

var kubectl kc.Kubectl

func init() {
	var err error
	kubectl, err = kc.NewKubectl()
	if err != nil {
		panic(err)
	}
	Cleanup()
	res := kubectl.CreateResource(kc.ResourceNamespace, testNamespace, kc.KubectlOptions{})
	if !res.WasSuccess() {
		panic(fmt.Sprintf("err: %v\n%s", res.Error(), res.StdErr()))
	}
}

func TestTests(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting test suite\n")
	RunSpecs(t, "Tests")
	Cleanup()
}

func Cleanup() {
	kubectl.DeleteResource(kc.ResourceNamespace, testNamespace, kc.KubectlOptions{})
}
