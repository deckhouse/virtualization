package e2e

import (
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

type ApplyWaitGetOptions struct {
	WaitTimeout time.Duration
	Phase       string
}

func ItApplyWaitGet(filepath string, options ApplyWaitGetOptions) {
	GinkgoHelper()
	timeout := ShortWaitDuration
	if options.WaitTimeout != 0 {
		timeout = options.WaitTimeout
	}
	phase := PhaseReady
	if options.Phase != "" {
		phase = options.Phase
	}
	ItApplyFromFile(filepath)
	ItWaitFromFile(filepath, phase, timeout)
	ItChekStatusPhaseFromFile(filepath, phase)

}

func ItApplyFromFile(filepath string) {
	GinkgoHelper()
	It("Apply resource from file", func() {
		ApplyFromFile(filepath)
	})

}
func ApplyFromFile(filepath string) {
	GinkgoHelper()
	fmt.Printf("Apply file %s\n", filepath)
	res := kubectl.Apply(filepath, kc.ApplyOptions{})
	Expect(res.Error()).To(BeNil(), "apply failed for file %s\n%s", filepath, res.StdErr())
}

func ItWaitFromFile(filepath, phase string, timeout time.Duration) {
	GinkgoHelper()
	It("Wait resource", func() {
		WaitFromFile(filepath, phase, timeout)
	})
}
func WaitFromFile(filepath, phase string, timeout time.Duration) {
	GinkgoHelper()
	For := "jsonpath={.status.phase}=" + phase
	res := kubectl.Wait(filepath, kc.WaitOptions{
		Timeout: timeout,
		For:     For,
	})
	Expect(res.Error()).To(BeNil(), "wait failed for file %s\n%s", filepath, res.StdErr())
}

func ItChekStatusPhaseFromFile(filepath, phase string) {
	GinkgoHelper()
	out := "jsonpath={.status.phase}"
	ItCheckStatusFromFile(filepath, out, phase)
}

func ItCheckStatusFromFile(filepath, output, compareField string) {
	GinkgoHelper()
	unstructs, err := helper.ParseYaml(filepath)
	Expect(err).To(BeNil(), "cannot decode objs from yaml file %s", filepath)

	for _, u := range unstructs {
		It("Get recourse status "+u.GetName(), func() {
			fullName := helper.GetFullApiResourceName(u)
			var res *executor.CMDResult
			if u.GetNamespace() == "" {
				res = kubectl.GetResource(fullName, u.GetName(), kc.GetOptions{Output: output})
			} else {
				res = kubectl.GetResource(fullName, u.GetName(), kc.GetOptions{
					Output:    output,
					Namespace: u.GetNamespace()})
			}
			Expect(res.Error()).To(BeNil(),
				"get failed resource %s %s/%s.\n%s",
				u.GetKind(),
				u.GetNamespace(),
				u.GetName(),
				res.StdErr(),
			)
			Expect(res.StdOut()).To(Equal(compareField))
		})
	}
}

func WaitResource(resource kc.Resource, name, For string, timeout time.Duration) {
	GinkgoHelper()
	res := kubectl.WaitResource(resource, name, kc.WaitOptions{
		Namespace: conf.Namespace,
		For:       For,
		Timeout:   timeout,
	})
	Expect(res.Error()).To(BeNil(), "wait failed %s %s/%s.\n%s", resource, conf.Namespace, name, res.StdErr())
}

func PatchResource(resource kc.Resource, name string, patch *kc.JsonPatch) {
	GinkgoHelper()
	res := kubectl.PatchResource(resource, name, kc.PatchOptions{
		Namespace: conf.Namespace,
		JsonPatch: patch,
	})
	Expect(res.Error()).To(BeNil(), "patch failed %s %s/%s.\n%s", resource, conf.Namespace, name, res.StdErr())
}
func CheckField(resource kc.Resource, name, output, compareValue string) {
	GinkgoHelper()
	res := kubectl.GetResource(resource, name, kc.GetOptions{
		Namespace: conf.Namespace,
		Output:    output,
	})
	Expect(res.Error()).To(BeNil(), "get failed %s %s/%s.\n%s", resource, conf.Namespace, name, res.StdErr())
	Expect(res.StdOut()).To(Equal(compareValue))
}
