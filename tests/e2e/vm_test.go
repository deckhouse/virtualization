package e2e

import (
	"encoding/json"
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	virt "github.com/deckhouse/virtualization/tests/e2e/virtctl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"io/fs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	KubeVirtVMStatusStopped   = "Stopped"
	KubeVirtVMStatusRunning   = "Running"
	KubeVirtRunStrategyAlways = "Always"
	KubeVirtRunStrategyHalted = "Halted"
	VMStatusRunning           = "Running"
	RunPolicyAlwaysOn         = "AlwaysOn"
	RunPolicyAlwaysOff        = "AlwaysOff"
)

func vmPath(file string) string {
	return path.Join(conf.VM.TestDataDir, file)
}

var _ = Describe("VM", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
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

	WaitKubevirtVMStatus := func(name, printableStatus string) {
		GinkgoHelper()
		WaitResource(kc.ResourceKubevirtVM, name, "jsonpath={.status.printableStatus}="+printableStatus, LongWaitDuration)
	}

	WaitVmStatus := func(name, phase string) {
		GinkgoHelper()
		WaitResource(kc.ResourceVM, name, "jsonpath={.status.phase}="+phase, LongWaitDuration)
	}

	GetVmStatus := func(name, phase string) {
		GinkgoHelper()
		CheckField(kc.ResourceVM, name, "jsonpath={.status.phase}", phase)
	}

	Context("Boot", func() {
		AfterAll(func() {
			kubectl.Delete(vmPath("boot/"), kc.DeleteOptions{})
		})
		Test := func(manifest string) {
			GinkgoHelper()
			var name string
			BeforeAll(func() {
				vm, err := GetVMFromManifest(manifest)
				Expect(err).To(BeNil())
				name = vm.Name
			})
			ItApplyFromFile(manifest)
			It("Wait vm running", func() {
				WaitVmStatus(name, VMStatusRunning)
			})
			It("Check vm phase", func() {
				GetVmStatus(name, VMStatusRunning)
			})
		}
		When("VMI source", func() {
			manifest := vmPath("boot/vm_vmi.yaml")
			Test(manifest)
		})
		When("CVMI source", func() {
			manifest := vmPath("boot/vm_cvmi.yaml")
			Test(manifest)
		})
		When("VMD source", func() {
			manifest := vmPath("boot/vm_vmd.yaml")
			Test(manifest)
		})
	})

	Context("RunPolicy", func() {
		manifest := vmPath("vm_runpolicy.yaml")
		var name string

		PatchVmRunPolicy := func(name, runPolicy string) {
			GinkgoHelper()
			PatchResource(kc.ResourceVM, name, &kc.JsonPatch{
				Op:    "replace",
				Path:  "/spec/runPolicy",
				Value: runPolicy,
			})
		}
		GetKubevirtRunStrategy := func(name, strategy string) {
			GinkgoHelper()
			output := "jsonpath={.spec.runStrategy}"
			CheckField(kc.ResourceKubevirtVM, name, output, strategy)
		}
		BeforeAll(func() {
			By("Apply manifest")
			vm, err := GetVMFromManifest(manifest)
			Expect(err).To(BeNil())
			name = vm.Name
			ApplyFromFile(manifest)
			WaitVmStatus(name, VMStatusRunning)
		})
		AfterAll(func() {
			By("Delete manifest")
			kubectl.Delete(manifest, kc.DeleteOptions{})
		})
		When("On to AlwaysOff", func() {
			It("Patch runpolicy to AlwaysOff", func() {
				PatchVmRunPolicy(name, RunPolicyAlwaysOff)
			})
			It("Wait kubevirt vm stopped", func() {
				WaitKubevirtVMStatus(name, KubeVirtVMStatusStopped)
			})
			It("Get kubevirt vm", func() {
				GetKubevirtRunStrategy(name, KubeVirtRunStrategyHalted)
			})
		})
		When("Virtctl start", func() {
			It("Virtctl start", func() {
				res := virtctl.StartVm(name, conf.Namespace)
				Expect(res.Error()).To(BeNil(), "virtctl start failed vm %s/%s.\n%s", conf.Namespace, name, res.StdErr())
			})
			It("Get kubevirt vm", func() {
				time.Sleep(30 * time.Second)
				GetKubevirtRunStrategy(name, KubeVirtRunStrategyHalted)
			})
		})
		When("Off to AlwaysOn", func() {
			It("Patch runpolicy to AlwaysOn", func() {
				PatchVmRunPolicy(name, RunPolicyAlwaysOn)
			})
			It("Wait kubevirt vm running", func() {
				WaitKubevirtVMStatus(name, KubeVirtVMStatusRunning)
			})
			It("Get kubevirt vm", func() {
				GetKubevirtRunStrategy(name, KubeVirtRunStrategyAlways)
			})
		})
		When("Virtctl stop", func() {
			It("Virtctl stop", func() {
				res := virtctl.StopVm(name, conf.Namespace)
				Expect(res.Error()).To(BeNil(), "virtctl stop failed vm %s/%s.\n%s", conf.Namespace, name, res.StdErr())
			})
			It("Get kubevirt vm", func() {
				time.Sleep(30 * time.Second)
				GetKubevirtRunStrategy(name, KubeVirtRunStrategyAlways)
			})
		})

	})

	Context("Provisioning", func() {
		CheckSsh := func(vmName string) {
			GinkgoHelper()
			res := virtctl.SshCommand(vmName, "sudo whoami", virt.SshOptions{
				Namespace:   conf.Namespace,
				Username:    "user",
				IdenityFile: vmPath("provisioning/id_ed"),
			})
			Expect(res.Error()).To(BeNil(), "check ssh failed for %s/%s.\n%s", conf.Namespace, vmName, res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).To(Equal("root"))
		}

		Test := func(manifest string) {
			GinkgoHelper()
			var name string
			BeforeAll(func() {
				vm, err := GetVMFromManifest(manifest)
				Expect(err).To(BeNil())
				name = vm.Name
			})
			ItApplyFromFile(manifest)
			It("Wait vm running", func() {
				WaitVmStatus(name, VMStatusRunning)
			})
			It("Check ssh", func() {
				CheckSsh(name)
			})
		}
		AfterAll(func() {
			By("Delete manifests")
			kubectl.Delete(vmPath("provisioning/"), kc.DeleteOptions{})
		})
		When("UserData", func() {
			manifest := vmPath("provisioning/vm_provisioning_useradata.yaml")
			Test(manifest)
		})
		When("UserDataSecretRef", func() {
			manifest := vmPath("provisioning/vm_provisioning_secret.yaml")
			Test(manifest)
		})
	})

	Context("Network", func() {

	})

	Context("Resources", func() {
		GetKubevirtResources := func(name string) (*corev1.ResourceRequirements, error) {
			GinkgoHelper()
			res := kubectl.GetResource(kc.ResourceKubevirtVM, name, kc.GetOptions{
				Output:    "jsonpath={.spec.template.spec.domain.resources}",
				Namespace: conf.Namespace,
			})
			if !res.WasSuccess() {
				return nil, fmt.Errorf("err: %w. %s", res.Error(), res.StdErr())
			}
			var resources corev1.ResourceRequirements
			if err := json.Unmarshal(res.StdOutBytes(), &resources); err != nil {
				return nil, err
			}
			return &resources, nil
		}
		CompareLimits := func(resKubevirt *corev1.ResourceRequirements) {
			GinkgoHelper()
			Expect(resKubevirt.Limits.Cpu().String()).To(Equal("1"))
			Expect(resKubevirt.Limits.Memory().String()).To(Equal("1Gi"))
		}
		CompareRequrest := func(resKubevirt *corev1.ResourceRequirements, cpu string, mem string) {
			GinkgoHelper()
			Expect(resKubevirt.Requests.Cpu().String()).To(Equal(cpu))
			Expect(resKubevirt.Requests.Memory().String()).To(Equal(mem))
		}

		Test := func(manifest, cpuPer, memPer string) {
			GinkgoHelper()
			var name string
			var kubevirtResources *corev1.ResourceRequirements
			BeforeAll(func() {
				vm, err := GetVMFromManifest(manifest)
				Expect(err).To(BeNil())
				name = vm.Name
				ApplyFromFile(manifest)
				WaitFromFile(manifest, VMStatusRunning, LongWaitDuration)
				kubevirtResources, err = GetKubevirtResources(name)
				Expect(err).To(BeNil())
			})
			It("Compare limit from Vm and Kubevirt", func() {
				CompareLimits(kubevirtResources)
			})
			It("Comprare request limit from VmKubevirt", func() {
				CompareRequrest(kubevirtResources, cpuPer, memPer)
			})
		}
		AfterAll(func() {
			By("Delete manifests")
			kubectl.Delete(vmPath("resources/"), kc.DeleteOptions{})
		})
		When("Corefraction 100", func() {
			manifest := vmPath("resources/vm_100.yaml")
			Test(manifest, "1", "1Gi")
		})
		When("Corefraction 50", func() {
			manifest := vmPath("resources/vm_50.yaml")
			Test(manifest, "500m", "1Gi")
		})
		When("Corefraction 25", func() {
			manifest := vmPath("resources/vm_25.yaml")
			Test(manifest, "250m", "1Gi")
		})
	})

	Context("NodePlacement", func() {

	})

	Context("PriorityClassName", func() {
		manifest := vmPath("vm_priorityclassname.yaml")
		var name string
		var class string

		GetKubevirtPriorityClassName := func(name, priorityClassName string) {
			GinkgoHelper()
			output := "jsonpath={.spec.template.spec.priorityClassName}"
			CheckField(kc.ResourceKubevirtVM, name, output, priorityClassName)
		}

		BeforeAll(func() {
			By("Apply manifest")
			vm, err := GetVMFromManifest(manifest)
			Expect(err).To(BeNil(), "failed parse manifest %s", manifest)
			name = vm.Name
			class = vm.Spec.PriorityClassName

			ApplyFromFile(manifest)
			WaitVmStatus(name, VMStatusRunning)
		})
		AfterAll(func() {
			By("Delete manifests")
			kubectl.Delete(manifest, kc.DeleteOptions{})
		})
		When("Compare priorityClassNames", func() {
			It("Compare priorityClassNames", func() {
				GetKubevirtPriorityClassName(name, class)
			})
		})
	})

	Context("TerminationGracePeriod", func() {
		manifest := vmPath("vm_graceperiod.yaml")
		jsonpath := "jsonpath={.spec.template.spec.terminationGracePeriodSeconds}"
		var name string
		var terminationGracePeriod string
		var patchTerminationGracePeriod string

		GetKubevirtGracePeriod := func(name, period string) {
			GinkgoHelper()
			output := jsonpath
			CheckField(kc.ResourceKubevirtVM, name, output, period)
		}

		BeforeAll(func() {
			By("Apply manifest")
			vm, err := GetVMFromManifest(manifest)
			Expect(err).To(BeNil(), "failed parse manifest %s.", manifest)
			name = vm.Name
			terminationGracePeriod = strconv.FormatInt(*vm.Spec.TerminationGracePeriodSeconds, 10)
			patchTerminationGracePeriod = strconv.FormatInt(*vm.Spec.TerminationGracePeriodSeconds+1, 10)
			ApplyFromFile(manifest)
			WaitVmStatus(name, VMStatusRunning)
		})
		AfterAll(func() {
			By("Delete manifest")
			kubectl.Delete(manifest, kc.DeleteOptions{})
		})
		When("Compare periods", func() {
			It("Compare periods", func() {
				GetKubevirtGracePeriod(name, terminationGracePeriod)
			})
		})
		When("Compare periods after patch", func() {
			It("Patch period", func() {
				PatchResource(kc.ResourceVM, name, &kc.JsonPatch{
					Op:    "replace",
					Path:  "/spec/terminationGracePeriodSeconds",
					Value: patchTerminationGracePeriod,
				})
			})
			It("Wait patch", func() {
				For := jsonpath + "=" + patchTerminationGracePeriod
				WaitResource(kc.ResourceKubevirtVM, name, For, LongWaitDuration)
			})
			It("Compare periods", func() {
				GetKubevirtGracePeriod(name, patchTerminationGracePeriod)
			})
		})
	})

})

type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualMachineSpec `json:"spec"`
}

type VirtualMachineSpec struct {
	RunPolicy                        RunPolicy           `json:"runPolicy"`
	VirtualMachineIPAddressClaimName string              `json:"virtualMachineIPAddressClaimName,omitempty"`
	NodeSelector                     map[string]string   `json:"nodeSelector,omitempty"`
	PriorityClassName                string              `json:"priorityClassName"`
	Tolerations                      []corev1.Toleration `json:"tolerations,omitempty"`
	TerminationGracePeriodSeconds    *int64              `json:"terminationGracePeriodSeconds,omitempty"`
	EnableParavirtualization         bool                `json:"enableParavirtualization,omitempty"`

	ApprovedChangeID string `json:"approvedChangeID,omitempty"`
}

type RunPolicy string

func GetVMFromManifest(manifest string) (*VirtualMachine, error) {
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
	var vm VirtualMachine
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}
