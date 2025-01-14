/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	UploadHelpPod = "upload-helper"
)

func cviPath(file string) string {
	return path.Join(conf.Disks.CviTestDataDir, file)
}

func viPath(file string) string {
	return path.Join(conf.Disks.ViTestDataDir, file)
}

func vdPath(file string) string {
	return path.Join(conf.Disks.VdTestDataDir, file)
}

var _ = Describe("Disks", ginkgoutil.CommonE2ETestDecorators(), func() {
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

	Context("CVI", func() {
		AfterAll(func() {
			By("Removing resources for cvi tests")
			kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{conf.Disks.CviTestDataDir},
				FilenameOption: kc.Filename,
			})
		})
		When("http source", func() {
			filepath := cviPath("/cvi_http.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("containerimage source", func() {
			filepath := cviPath("/cvi_containerimage.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("vi source", func() {
			filepath := cviPath("/cvi_vi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("cvi source", func() {
			filepath := cviPath("/cvi_cvi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for cvi upload test")
				kubectl.Delete(kc.DeleteOptions{
					Filename:  []string{UploadHelpPod},
					Namespace: conf.Namespace,
					Resource:  kc.ResourcePod,
				})
			})
			filepath := cviPath("/cvi_upload.yaml")
			ItUpload(filepath)
			CheckProgress(filepath)
		})
	})
	Context("VI", func() {
		AfterAll(func() {
			By("Removing resources for vi tests")
			kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{conf.Disks.ViTestDataDir},
				FilenameOption: kc.Filename,
			})
		})
		When("http source", func() {
			filepath := viPath("/vi_http.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("containerimage source", func() {
			filepath := viPath("/vi_containerimage.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{})
			CheckProgress(filepath)
		})
		When("vi source", func() {
			filepath := viPath("/vi_vi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("cvi source", func() {
			filepath := viPath("/vi_cvi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for vi upload test")
				kubectl.Delete(kc.DeleteOptions{
					Filename:  []string{UploadHelpPod},
					Namespace: conf.Namespace,
					Resource:  kc.ResourcePod,
				})
			})
			filepath := viPath("/vi_upload.yaml")
			ItUpload(filepath)
			CheckProgress(filepath)
		})
	})
	Context("VD", func() {
		AfterAll(func() {
			By("Removing resources for vd tests")
			kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{conf.Disks.VdTestDataDir},
				FilenameOption: kc.Filename,
			})
		})
		When("http source", func() {
			filepath := vdPath("/vd_http.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("containerimage source", func() {
			filepath := vdPath("/vd_containerimage.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("vi source", func() {
			filepath := vdPath("/vd_vi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("cvi source", func() {
			filepath := vdPath("/vd_cvi.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("blank", func() {
			filepath := vdPath("/vd_blank.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{
				WaitTimeout: LongWaitDuration,
			})
			CheckProgress(filepath)
		})
		When("upload", func() {
			AfterAll(func() {
				By("Removing support resources for vd upload test")
				kubectl.Delete(kc.DeleteOptions{
					Filename:  []string{UploadHelpPod},
					Namespace: conf.Namespace,
					Resource:  kc.ResourcePod,
				})
			})
			filepath := vdPath("/vd_upload.yaml")
			ItUpload(filepath)
			CheckProgress(filepath)
		})
	})
})
