package e2e

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type virtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func getVMFromManifest(manifest string) (*virtualMachine, error) {
	unstructs, err := helper.ParseYaml(manifest)
	if err != nil {
		return nil, err
	}
	var unstruct *unstructured.Unstructured
	for _, u := range unstructs {
		if helper.GetFullApiResourceName(u) == kc.ResourceVM {
			unstruct = u
			break
		}
	}
	var vm virtualMachine

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

var _ = Describe("Label and Annotation", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
	vmManifest := vmPath("vm_label_annotation.yaml")

	const (
		labelName       = "os"
		labelValue      = "ubuntu"
		annotationName  = "test-annotation"
		annotationValue = "true"
	)

	getPodName := func(resourceName string) string {
		getPodCMD := "get pod --no-headers -o custom-columns=':metadata.name'"
		subCMD := fmt.Sprintf("-n %s %s | grep %s", conf.Namespace, getPodCMD, resourceName)
		podCMD := kubectl.RawCommand(subCMD, ShortWaitDuration)
		podName := strings.TrimSuffix(podCMD.StdOut(), "\n")
		return podName
	}

	getRecourseLabel := func(resourceType kc.Resource, resourceName string) *executor.CMDResult {
		label := fmt.Sprintf("jsonpath='{.metadata.labels.%s}'", labelName)
		cmdResult := kubectl.GetResource(resourceType, resourceName, kc.GetOptions{
			Output:    label,
			Namespace: conf.Namespace,
		})
		return cmdResult
	}

	getRecourseAnnotation := func(resourceType kc.Resource, resourceName string) *executor.CMDResult {
		annotation := fmt.Sprintf("jsonpath='{.metadata.annotations.%s}'", annotationName)
		cmdResult := kubectl.GetResource(resourceType, resourceName, kc.GetOptions{
			Output:    annotation,
			Namespace: conf.Namespace,
		})
		return cmdResult
	}

	WaitVmStatus := func(name, phase string) {
		GinkgoHelper()
		WaitResource(kc.ResourceVM, name, "jsonpath={.status.phase}="+phase, LongWaitDuration)
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

	Context("Label", func() {
		var vm *virtualMachine
		var err error

		BeforeAll(func() {
			By("Apply manifest")
			vm, err = getVMFromManifest(vmManifest)
			Expect(err).NotTo(HaveOccurred())
			ApplyFromFile(vmManifest)
			WaitVmStatus(vm.Name, VMStatusRunning)
		})

		AfterAll(func() {
			By("Delete manifest")
			kubectl.Delete(vmManifest, kc.DeleteOptions{})
		})

		Describe(fmt.Sprintf("Add label %s=%s", labelName, labelValue), func() {
			It("Labeled", func() {
				subCMD := fmt.Sprintf("-n %s label vm %s %s=%s", conf.Namespace, vm.Name, labelName, labelValue)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})

		Describe("Check label on resource", func() {

			It("VM", func() {
				res := getRecourseLabel(kc.ResourceVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get VM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(labelValue))
			})
			It("KVVM", func() {
				res := getRecourseLabel(kc.ResourceKubevirtVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(labelValue))
			})
			It("KVVMI", func() {
				res := getRecourseLabel(kc.ResourceKubevirtVMI, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVMI %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(labelValue))
			})
			It("POD virtlauncher", func() {
				pod := getPodName(vm.Name)
				res := getRecourseLabel(kc.ResourcePod, pod)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get pod %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(labelValue))
			})
		})

		Describe(fmt.Sprintf("Remove label %s=%s", labelName, labelValue), func() {

			It("Label was removed", func() {
				subCMD := fmt.Sprintf("-n %s label vm %s %s-", conf.Namespace, vm.Name, labelName)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})

		Describe("Label must be removed from resource", func() {

			It("VM", func() {
				res := getRecourseLabel(kc.ResourceVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get VM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVM", func() {
				res := getRecourseLabel(kc.ResourceKubevirtVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVMI", func() {
				res := getRecourseLabel(kc.ResourceKubevirtVMI, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVMI %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("POD virtlauncher", func() {
				pod := getPodName(vm.Name)
				res := getRecourseLabel(kc.ResourcePod, pod)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get pod %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
		})
	})

	Context("Annotation", func() {
		var vm *virtualMachine

		BeforeAll(func() {
			By("Apply manifest")
			var err error
			vm, err = getVMFromManifest(vmManifest)
			Expect(err).NotTo(HaveOccurred())
			ApplyFromFile(vmManifest)
			WaitVmStatus(vm.Name, VMStatusRunning)
		})

		AfterAll(func() {
			By("Delete manifest")
			kubectl.Delete(vmManifest, kc.DeleteOptions{})
		})

		Describe(fmt.Sprintf("Add annotation %s=%s", annotationName, annotationValue), func() {
			It("Annotated", func() {
				subCMD := fmt.Sprintf("-n %s annotate vm %s %s=%s", conf.Namespace, vm.Name, annotationName, annotationValue)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})

		Describe("Check annotation on resource", func() {
			It("VM", func() {
				res := getRecourseAnnotation(kc.ResourceVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get VM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(annotationValue))
			})
			It("KVVM", func() {
				res := getRecourseAnnotation(kc.ResourceKubevirtVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(annotationValue))
			})
			It("KVVMI", func() {
				res := getRecourseAnnotation(kc.ResourceKubevirtVMI, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVMI %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(annotationValue))
			})
			It("POD virtlauncher", func() {
				pod := getPodName(vm.Name)
				res := getRecourseAnnotation(kc.ResourcePod, pod)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get pod %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(Equal(annotationValue))
			})
		})

		Describe("Remove annotation test-annotation=true", func() {

			It("Was removed", func() {
				subCMD := fmt.Sprintf("-n %s annotate vm %s %s-", conf.Namespace, vm.Name, annotationName)
				res := kubectl.RawCommand(subCMD, ShortWaitDuration)
				Expect(res.Error()).NotTo(HaveOccurred())
			})
		})

		Describe("Annotation must be removed from resource", func() {
			It("VM", func() {
				res := getRecourseAnnotation(kc.ResourceVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get VM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVM", func() {
				res := getRecourseAnnotation(kc.ResourceKubevirtVM, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVM %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("KVVMI", func() {
				res := getRecourseAnnotation(kc.ResourceKubevirtVMI, vm.Name)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get KVVMI %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
			It("POD virtlauncher", func() {
				pod := getPodName(vm.Name)
				res := getRecourseAnnotation(kc.ResourcePod, pod)
				Expect(res.Error()).NotTo(HaveOccurred(), "failed to get pod %s.\n%s", vm.Name, res.StdErr())
				Expect(res.StdOut()).To(BeEmpty())
			})
		})
	})
})
