package e2e

import (
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	virt "github.com/deckhouse/virtualization/tests/e2e/virtctl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

const (
	ShortWaitDuration      = 60 * time.Second
	LongWaitDuration       = 300 * time.Second
	PhaseReady             = "Ready"
	PhaseSucceeded         = "Succeeded"
	PhaseWaitForUserUpload = "WaitForUserUpload"
)

var (
	conf    *config.Config
	kubectl kc.Kubectl
	virtctl virt.Virtctl
)

func init() {
	var err error
	if conf, err = config.GetConfig(); err != nil {
		panic(err)
	}
	if kubectl, err = kc.NewKubectl(kc.KubectlConf(conf.ClusterTransport)); err != nil {
		panic(err)
	}
	if virtctl, err = virt.NewVirtctl(virt.VirtctlConf(conf.ClusterTransport)); err != nil {
		panic(err)
	}
	Cleanup()
	res := kubectl.CreateResource(kc.ResourceNamespace, conf.Namespace, kc.CreateOptions{})
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
	kubectl.DeleteResource(kc.ResourceNamespace, conf.Namespace, kc.DeleteOptions{})
}
