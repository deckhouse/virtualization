package e2e_test

import (
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

const (
	CVMITestdataDir = TestdataDir + "/cvmi"
	VMITestdataDir  = TestdataDir + "/vmi"
	VMDTestdataDir  = TestdataDir + "/vmd"
	UploadHelpImage = "yaroslavborbat/curl-alpine-image"
	UploadHelpPod   = "upload-helper"
)

var _ = Describe("Disks", func() {
	BeforeEach(func() {
		By("Check if kubectl can connect to cluster")

		res := kubectl.List(kc.ResourceNode, kc.KubectlOptions{
			Output: "jsonpath={.items[0].status.conditions[-1].type}",
		})

		Expect(res.StdOut()).To(Equal("Ready"))
	})

	Context("CVMI", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for cvmi tests")
			kubectl.Delete(CVMITestdataDir+"/", kc.KubectlOptions{})
		})
		When("http source", func() {
			ApplyWaitGetReady(CVMITestdataDir+"/cvmi_http.yaml", ApplyWaitGetReadyOptions{})
		})
		When("containerimage source", func() {
			ApplyWaitGetReady(CVMITestdataDir+"/cvmi_containerimage.yaml", ApplyWaitGetReadyOptions{})
		})
		When("vmi source", func() {
			ApplyWaitGetReady(CVMITestdataDir+"/cvmi_vmi.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("cvmi source", func() {
			ApplyWaitGetReady(CVMITestdataDir+"/cvmi_cvmi.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for cvmi upload test")
				kubectl.DeleteResource(kc.ResourcePod, UploadHelpPod, kc.KubectlOptions{
					Namespace: testNamespace,
				})
			})
			Upload(CVMITestdataDir + "/cvmi_upload.yaml")
		})
	})
	Context("VMI", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for vmi tests")
			kubectl.Delete(VMITestdataDir+"/", kc.KubectlOptions{})
		})
		When("http source", func() {
			ApplyWaitGetReady(VMITestdataDir+"/vmi_http.yaml", ApplyWaitGetReadyOptions{})
		})
		When("containerimage source", func() {
			ApplyWaitGetReady(VMITestdataDir+"/vmi_containerimage.yaml", ApplyWaitGetReadyOptions{})
		})
		When("vmi source", func() {
			ApplyWaitGetReady(VMITestdataDir+"/vmi_vmi.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("cvmi source", func() {
			ApplyWaitGetReady(VMITestdataDir+"/vmi_cvmi.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for vmi upload test")
				kubectl.DeleteResource(kc.ResourcePod, UploadHelpPod, kc.KubectlOptions{
					Namespace: testNamespace,
				})
			})
			Upload(VMITestdataDir + "/vmi_upload.yaml")
		})
	})
	Context("VMD", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for vmd tests")
			kubectl.Delete(VMDTestdataDir+"/", kc.KubectlOptions{})
		})
		When("http source", func() {
			ApplyWaitGetReady(VMDTestdataDir+"/vmd_http.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("containerimage source", func() {
			ApplyWaitGetReady(VMDTestdataDir+"/vmd_containerimage.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("vmi source", func() {
			ApplyWaitGetReady(VMDTestdataDir+"/vmd_vmi.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("cvmi source", func() {
			ApplyWaitGetReady(VMDTestdataDir+"/vmd_cvmi.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("blank", func() {
			ApplyWaitGetReady(VMDTestdataDir+"/vmd_blank.yaml", ApplyWaitGetReadyOptions{
				WaitTimeout: LongWaitDuration,
			})
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for vmd upload test")
				kubectl.DeleteResource(kc.ResourcePod, UploadHelpPod, kc.KubectlOptions{
					Namespace: testNamespace,
				})
			})
			Upload(VMDTestdataDir + "/vmd_upload.yaml")
		})
	})
})

type ApplyWaitGetReadyOptions struct {
	WaitTimeout time.Duration
	Phase       string
}

func ApplyWaitGetReady(filepath string, options ApplyWaitGetReadyOptions) {
	timeout := ShortWaitDuration
	if options.WaitTimeout != 0 {
		timeout = options.WaitTimeout
	}
	phase := PhaseReady
	if options.Phase != "" {
		phase = options.Phase
	}
	Apply(filepath)
	Wait(filepath, phase, timeout)
	Get(filepath, phase)

}

func Apply(filepath string) {
	It("Apply resource from file", func() {
		fmt.Printf("Run test for file %s\n", filepath)
		res := kubectl.Apply(filepath, kc.KubectlOptions{})
		if !res.WasSuccess() {
			defer GinkgoRecover()
			Fail(fmt.Sprintf("apply failed for file %s\n err: %v\n %s", filepath, res.Error(), res.StdErr()))
		}
	})

}

func Wait(filepath, phase string, timeout time.Duration) {
	It("Wait resource", func() {
		forF := "jsonpath={.status.phase}=" + phase
		res := kubectl.Wait(filepath, kc.KubectlOptions{
			WaitTimeout: timeout,
			WaitFor:     forF,
		})
		if !res.WasSuccess() {
			defer GinkgoRecover()
			Fail(fmt.Sprintf("wait failed for file %s\n err: %v\n %s", filepath, res.Error(), res.StdErr()))
		}
	})
}

func Get(filepath, phase string) {
	unstructs, err := helper.ParseYaml(filepath)
	if err != nil {
		defer GinkgoRecover()
		Fail(fmt.Sprintf("cannot decode objs from yaml file %s %v", filepath, err))
	}

	for _, u := range unstructs {
		It("Get recourse status "+u.GetName(), func() {
			fullName := helper.GetFullApiResourceName(u)
			out := "jsonpath={.status.phase}"
			var res *executor.CMDResult
			if u.GetNamespace() == "" {
				res = kubectl.GetResource(fullName, u.GetName(), kc.KubectlOptions{Output: out})
			} else {
				res = kubectl.GetResource(fullName, u.GetName(), kc.KubectlOptions{
					Output:    out,
					Namespace: u.GetNamespace()})
			}
			if !res.WasSuccess() {
				defer GinkgoRecover()
				Fail(fmt.Sprintf("get failed resource %s %s/%s\n err: %v\n%s",
					u.GetKind(),
					u.GetNamespace(),
					err,
					u.GetName(),
					res.StdErr(),
				),
				)
			}
			Expect(res.StdOut()).To(Equal(phase))
		})
	}
}

func Upload(filepath string) {
	ApplyWaitGetReady(filepath, ApplyWaitGetReadyOptions{
		Phase: PhaseWaitForUserUpload,
	})
	It("Run pod upload helper", func() {
		res := kubectl.Get(filepath, kc.KubectlOptions{
			Output: "jsonpath={.status.uploadCommand}",
		})
		if !res.WasSuccess() {
			defer GinkgoRecover()
			Fail(fmt.Sprintf("get failed upload. err: %v\n%s", res.Error(), res.StdErr()))
		}
		subCMD := fmt.Sprintf("run -n %s --restart=Never -i --tty %s --image=%s -- %s", testNamespace, UploadHelpPod, UploadHelpImage, res.StdOut())
		res = kubectl.RawCommand(subCMD, LongWaitDuration)
		if !res.WasSuccess() {
			defer GinkgoRecover()
			Fail(fmt.Sprintf("craete pod upload helper failed. err: %v\n%s", res.Error(), res.StdErr()))
		}
		forF := "jsonpath={.status.phase}=" + PhaseSucceeded
		res = kubectl.WaitResource(kc.ResourcePod, UploadHelpPod, kc.KubectlOptions{
			WaitFor:     forF,
			WaitTimeout: LongWaitDuration,
		})
		if !res.WasSuccess() {
			defer GinkgoRecover()
			Fail(fmt.Sprintf("wait failed pod %s/%s. err: %v\n%s", testNamespace, UploadHelpPod, res.Error(), res.StdErr()))
		}
		res = kubectl.GetResource(kc.ResourcePod, UploadHelpPod,
			kc.KubectlOptions{
				Namespace: testNamespace,
				Output:    "jsonpath={.status.phase}",
			})
		if !res.WasSuccess() {
			defer GinkgoRecover()
			Fail(fmt.Sprintf("get failed pod %s/%s. err: %v\n%s", testNamespace, UploadHelpPod, res.Error(), res.StdErr()))
		}
		Expect(res.StdOut()).To(Equal(PhaseSucceeded))
	})
	Wait(filepath, PhaseReady, ShortWaitDuration)
	Get(filepath, PhaseReady)
}
