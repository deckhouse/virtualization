package e2e

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	UploadHelpPod = "upload-helper"
)

func cvmiPath(file string) string {
	return path.Join(conf.Disks.CvmiTestDataDir, file)
}

func vmiPath(file string) string {
	return path.Join(conf.Disks.VmiTestDataDir, file)
}

func vmdPath(file string) string {
	return path.Join(conf.Disks.VmdTestDataDir, file)
}

var _ = Describe("Disks", func() {
	CheckProgress := func(filepath string) {
		GinkgoHelper()
		out := "jsonpath={.status.progress}"
		ItCheckStatusFromFile(filepath, out, "100%")
	}

	ItUpload := func(filepath string) {
		GinkgoHelper()
		ItApplyWaitGet(filepath, ApplyWaitGetOptions{
			Phase: PhaseWaitForUserUpload,
		})
		It("Run pod upload helper", func() {
			res := kubectl.Get(filepath, kc.GetOptions{
				Output: "jsonpath={.status.uploadCommand}",
			})

			Expect(res.Error()).NotTo(HaveOccurred(), "get failed upload.\n%s", res.StdErr())
			subCMD := fmt.Sprintf("run -n %s --restart=Never -i --tty %s --image=%s -- %s", conf.Namespace, UploadHelpPod, conf.Disks.UploadHelperImage, res.StdOut()+" -k")

			res = kubectl.RawCommand(subCMD, LongWaitDuration)
			Expect(res.Error()).NotTo(HaveOccurred(), "create pod upload helper failed.\n%s", res.StdErr())
			For := "jsonpath={.status.phase}=" + PhaseSucceeded
			WaitResource(kc.ResourcePod, UploadHelpPod, For, LongWaitDuration)
			CheckField(kc.ResourcePod, UploadHelpPod, "jsonpath={.status.phase}", PhaseSucceeded)
		})
		ItWaitFromFile(filepath, PhaseReady, ShortWaitDuration)
		ItChekStatusPhaseFromFile(filepath, PhaseReady)
	}

	Context("CVMI", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for cvmi tests")
			kubectl.Delete(conf.Disks.CvmiTestDataDir, kc.DeleteOptions{})
		})
		When("http source", func() {
			filepath := cvmiPath("/cvmi_http.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("containerimage source", func() {
			filepath := cvmiPath("/cvmi_containerimage.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("vmi source", func() {
			filepath := cvmiPath("/cvmi_vmi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("cvmi source", func() {
			filepath := cvmiPath("/cvmi_cvmi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for cvmi upload test")
				kubectl.DeleteResource(kc.ResourcePod, UploadHelpPod, kc.DeleteOptions{
					Namespace: conf.Namespace,
				})
			})
			filepath := cvmiPath("/cvmi_upload.yaml")
			ItUpload(filepath)
			CheckProgress(filepath)
		})
	})
	Context("VMI", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for vmi tests")
			kubectl.Delete(conf.Disks.VmiTestDataDir, kc.DeleteOptions{})
		})
		When("http source", func() {
			filepath := vmiPath("/vmi_http.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("containerimage source", func() {
			filepath := vmiPath("/vmi_containerimage.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("vmi source", func() {
			filepath := vmiPath("/vmi_vmi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("cvmi source", func() {
			filepath := vmiPath("/vmi_cvmi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for vmi upload test")
				kubectl.DeleteResource(kc.ResourcePod, UploadHelpPod, kc.DeleteOptions{
					Namespace: conf.Namespace,
				})
			})
			filepath := vmiPath("/vmi_upload.yaml")
			ItUpload(filepath)
			CheckProgress(filepath)
		})
	})
	Context("VMD", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for vmd tests")
			kubectl.Delete(conf.Disks.VmdTestDataDir, kc.DeleteOptions{})
		})
		When("http source", func() {
			filepath := vmdPath("/vmd_http.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("containerimage source", func() {
			filepath := vmdPath("/vmd_containerimage.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("vmi source", func() {
			filepath := vmdPath("/vmd_vmi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("cvmi source", func() {
			filepath := vmdPath("/vmd_cvmi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("blank", func() {
			filepath := vmdPath("/vmd_blank.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for vmd upload test")
				kubectl.DeleteResource(kc.ResourcePod, UploadHelpPod, kc.DeleteOptions{
					Namespace: conf.Namespace,
				})
			})
			filepath := vmdPath("/vmd_upload.yaml")
			ItUpload(filepath)
			CheckProgress(filepath)
		})
	})
})
