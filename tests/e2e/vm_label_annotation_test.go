package e2e

import (
	"fmt"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"io/fs"
	"path/filepath"
	"strings"
)

var _ = Describe("Label and Annotation", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
	manifestVM := vmPath("vm_label_annotation.yaml")

	type jsonPathCMD struct {
		label      string
		annotation string
	}

	type resourceCMD struct {
		getVM       string
		getKVVM     string
		getKVVMi    string
		getPOD      string
		getJsonPath jsonPathCMD
	}

	kctlCMD := resourceCMD{
		getPOD:   "get pod --no-headers -o custom-columns=':metadata.name'",
		getVM:    "get vm",
		getKVVM:  "get virtualmachines.x.virtualization.deckhouse.io",
		getKVVMi: "get virtualmachineinstances.x.virtualization.deckhouse.io",
		getJsonPath: jsonPathCMD{
			label:      "-o jsonpath='{.metadata.labels.os}'",
			annotation: "-o jsonpath='{.metadata.annotations.test-annotation}'",
		},
	}

	BeforeAll(func() {
		By("Apply image for vms")
		ApplyFromFile(imageManifest)
		WaitFromFile(imageManifest, PhaseReady, LongWaitDuration)
	})
	AfterAll(func() {
		By("Delete all manifests")
		files := make([]string, 0)
		err := filepath.Walk(conf.VM.TestDataDir, func(path string, info fs.FileInfo, err error) error {
			if err == nil && strings.HasSuffix(info.Name(), "yaml") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil || len(files) == 0 {
			kubectl.Delete(imageManifest, kc.DeleteOptions{})
			kubectl.Delete(conf.VM.TestDataDir, kc.DeleteOptions{})
		} else {
			for _, f := range files {
				kubectl.Delete(f, kc.DeleteOptions{})
			}
		}
	})

	WaitVmStatus := func(name, phase string) {
		GinkgoHelper()
		WaitResource(kc.ResourceVM, name, "jsonpath={.status.phase}="+phase, LongWaitDuration)
	}

	Context("Label", func() {
		var name string

		label := "os=ubuntu"
		vm, err := GetVMFromManifest(manifestVM)

		BeforeAll(func() {
			By("Apply manifest")
			vm, err := GetVMFromManifest(manifestVM)
			Expect(err).To(BeNil())
			name = vm.Name
			ApplyFromFile(manifestVM)
			WaitVmStatus(name, VMStatusRunning)
		})

		AfterAll(func() {
			By("Delete manifest")
			kubectl.Delete(manifestVM, kc.DeleteOptions{})
		})

		Describe("Add label os=ubuntu", func() {

			It("Labeled", func() {
				Expect(err).To(BeNil())
				subCMD := fmt.Sprintf("-n %s label vm %s %s", conf.Namespace, vm.Name, label)

				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})
		Describe("Check label on resource", func() {
			It("VM", func() {
				subCMD := fmt.Sprintf("-n %s %s %s %s", conf.Namespace, kctlCMD.getVM, vm.Name, kctlCMD.getJsonPath.label)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("ubuntu"))
			})
			It("KVVM", func() {
				subCMD := fmt.Sprintf("-n %s %s %s %s", conf.Namespace, kctlCMD.getKVVM, vm.Name, kctlCMD.getJsonPath.label)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("ubuntu"))
			})
			It("KVVMI", func() {
				subCMD := fmt.Sprintf("-n %s %s %s %s", conf.Namespace, kctlCMD.getKVVMi, vm.Name, kctlCMD.getJsonPath.label)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("ubuntu"))
			})
			It("POD virtlauncher", func() {
				subCMD := fmt.Sprintf("-n %s %s | grep %s", conf.Namespace, kctlCMD.getPOD, vm.Name)
				podCMD := kubectl.RawCommand(subCMD, ShortWaitDuration)
				pod := strings.TrimSuffix(podCMD.StdOut(), "\n")

				subCMDPod := fmt.Sprintf("-n %s get po %s %s", conf.Namespace, pod, kctlCMD.getJsonPath.label)
				res := kubectl.RawCommand(subCMDPod, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("ubuntu"))
			})
		})
		Describe("Remove label os=ubuntu", func() {

			It("Label was removed", func() {
				Expect(err).To(BeNil())
				subCMD := fmt.Sprintf("-n %s label vm %s %s", conf.Namespace, vm.Name, "os-")

				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})
		Describe("Label must be removed from resource", func() {

			It("VM", func() {
				subCMD := fmt.Sprintf("-n %s get vm %s -o jsonpath='{.metadata.labels.os}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVM", func() {
				subCMD := fmt.Sprintf("-n %s get virtualmachines.x.virtualization.deckhouse.io %s -o jsonpath='{.metadata.labels.os}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVMI", func() {
				subCMD := fmt.Sprintf("-n %s get virtualmachineinstances.x.virtualization.deckhouse.io %s -o jsonpath='{.metadata.labels.os}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("POD virtlauncher", func() {
				subCMD := fmt.Sprintf("-n %s get pod --no-headers -o custom-columns=':metadata.name' | grep %s", conf.Namespace, vm.Name)
				podCMD := kubectl.RawCommand(subCMD, ShortWaitDuration)
				pod := strings.TrimSuffix(podCMD.StdOut(), "\n")

				subCMDPod := fmt.Sprintf("-n %s get po %s -o jsonpath='{.metadata.labels.os}'", conf.Namespace, pod)
				res := kubectl.RawCommand(subCMDPod, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
		})
	})

	Context("Annotation", func() {
		var name string

		annotation := "test-annotation=true"
		vm, err := GetVMFromManifest(manifestVM)

		BeforeAll(func() {
			By("Apply manifest")
			vm, err := GetVMFromManifest(manifestVM)
			Expect(err).To(BeNil())
			name = vm.Name
			ApplyFromFile(manifestVM)
			WaitVmStatus(name, VMStatusRunning)
		})

		AfterAll(func() {
			By("Delete manifest")
			kubectl.Delete(manifestVM, kc.DeleteOptions{})
		})

		Describe("Add annotation test-annotation=true", func() {

			It("Annotated", func() {
				Expect(err).To(BeNil())
				subCMD := fmt.Sprintf("-n %s annotate vm %s %s", conf.Namespace, vm.Name, annotation)

				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})
		Describe("Check annotation on resource", func() {
			It("VM", func() {
				subCMD := fmt.Sprintf("-n %s get vm %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("true"))
			})
			It("KVVM", func() {
				subCMD := fmt.Sprintf("-n %s get virtualmachines.x.virtualization.deckhouse.io %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("true"))
			})
			It("KVVMI", func() {
				subCMD := fmt.Sprintf("-n %s get virtualmachineinstances.x.virtualization.deckhouse.io %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("true"))
			})
			It("POD virtlauncher", func() {
				subCMD := fmt.Sprintf("-n %s get pod --no-headers -o custom-columns=':metadata.name' | grep %s", conf.Namespace, vm.Name)
				podCMD := kubectl.RawCommand(subCMD, ShortWaitDuration)
				pod := strings.TrimSuffix(podCMD.StdOut(), "\n")

				subCMDPod := fmt.Sprintf("-n %s get po %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, pod)
				res := kubectl.RawCommand(subCMDPod, ShortWaitDuration)
				Expect(res.StdOut()).To(Equal("true"))
			})
		})
		Describe("Remove annotation test-annotation=true", func() {

			It("Was removed", func() {
				Expect(err).To(BeNil())
				subCMD := fmt.Sprintf("-n %s annotate vm %s %s", conf.Namespace, vm.Name, "test-annotation-")

				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})
		Describe("Annotation must be removed from resource", func() {
			It("VM", func() {
				subCMD := fmt.Sprintf("-n %s get vm %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVM", func() {
				subCMD := fmt.Sprintf("-n %s get virtualmachines.x.virtualization.deckhouse.io %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVMI", func() {
				subCMD := fmt.Sprintf("-n %s get virtualmachineinstances.x.virtualization.deckhouse.io %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, vm.Name)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("POD virtlauncher", func() {
				subCMD := fmt.Sprintf("-n %s get pod --no-headers -o custom-columns=':metadata.name' | grep %s", conf.Namespace, vm.Name)
				podCMD := kubectl.RawCommand(subCMD, ShortWaitDuration)
				pod := strings.TrimSuffix(podCMD.StdOut(), "\n")

				subCMDPod := fmt.Sprintf("-n %s get po %s -o jsonpath='{.metadata.annotations.test-annotation}'", conf.Namespace, pod)
				res := kubectl.RawCommand(subCMDPod, ShortWaitDuration)
				Expect(res.StdOut()).To(BeEmpty())
			})
		})
	})
})
