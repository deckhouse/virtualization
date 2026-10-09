/*
Copyright 2026 Flant JSC

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

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmclassobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmclass"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

const (
	// The CPU features a guest hypervisor needs: vmx on Intel and svm on AMD.
	cpuFeatureVMX = "vmx"
	cpuFeatureSVM = "svm"

	cpuFeaturePolicyRequire = "require"
	cpuFeaturePolicyDisable = "disable"
)

var nestedVirtualizationCPUFeatures = []string{cpuFeatureVMX, cpuFeatureSVM}

// A VirtualMachineClass with nested virtualization off has to spell the features out as disabled:
// an unnamed feature is resolved by libvirt on its own, which for the CPU types that inherit the
// whole feature set from the node means vmx/svm stay on. The specs check both ends of that: the
// domain libvirt applied and the CPU flags the guest sees.
var _ = Describe("VirtualMachineClassNestedVirtualization", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("nested-virtualization")
		ctx = context.Background()
		DeferCleanup(f.After)
		f.Before()
	})

	DescribeTable("disables nested virtualization on a class that inherits the node features",
		func(cpuType v1alpha3.CPUType) {
			var class *v1alpha2.VirtualMachineClass
			By("Create the VirtualMachineClass with nested virtualization disabled", func() {
				class = createVMClass(ctx, f, newNestedVirtualizationVMClass(f, cpuType, false))
			})

			var vm *v1alpha2.VirtualMachine
			By("Start a VirtualMachine of the class", func() {
				vm = startVMOfClass(ctx, f, class.Name)
			})

			By("Verify the applied domain disables the nested virtualization features", func() {
				cpu := readDomainCPU(ctx, f, vm)
				for _, feature := range nestedVirtualizationCPUFeatures {
					expectCPUFeaturePolicy(cpu, feature, cpuFeaturePolicyDisable)
				}
			})

			By("Verify the guest sees no nested virtualization CPU flag", func() {
				flags := guestCPUFlags(f, vm)
				for _, feature := range nestedVirtualizationCPUFeatures {
					Expect(flags).NotTo(ContainElement(feature),
						"the guest of VirtualMachine %s/%s should not see the %q CPU flag", vm.Namespace, vm.Name, feature)
				}
			})
		},
		Entry("Host", v1alpha3.CPUTypeHost),
		Entry("HostPassthrough", v1alpha3.CPUTypeHostPassthrough),
	)

	Context("Discovery", func() {
		It("disables the nested virtualization features", func() {
			var class *v1alpha2.VirtualMachineClass
			By("Create the VirtualMachineClass with nested virtualization disabled", func() {
				class = createVMClass(ctx, f, newNestedVirtualizationVMClass(f, v1alpha3.CPUTypeDiscovery, false))
			})

			By("Verify the class leaves the nested virtualization features out of its CPU model", func() {
				for _, feature := range nestedVirtualizationCPUFeatures {
					Expect(class.Status.CpuFeatures.Enabled).NotTo(ContainElement(feature),
						"VirtualMachineClass %s should not list the %q CPU feature as enabled", class.Name, feature)
				}
			})

			var vm *v1alpha2.VirtualMachine
			By("Start a VirtualMachine of the class", func() {
				vm = startVMOfClass(ctx, f, class.Name)
			})

			By("Verify the applied domain disables the nested virtualization features", func() {
				cpu := readDomainCPU(ctx, f, vm)
				for _, feature := range nestedVirtualizationCPUFeatures {
					expectCPUFeaturePolicy(cpu, feature, cpuFeaturePolicyDisable)
				}
			})

			By("Verify the guest sees no nested virtualization CPU flag", func() {
				flags := guestCPUFlags(f, vm)
				for _, feature := range nestedVirtualizationCPUFeatures {
					Expect(flags).NotTo(ContainElement(feature),
						"the guest of VirtualMachine %s/%s should not see the %q CPU flag", vm.Namespace, vm.Name, feature)
				}
			})
		})

		It("requires the nested virtualization features its nodes provide", func() {
			var class *v1alpha2.VirtualMachineClass
			By("Create the VirtualMachineClass with nested virtualization enabled", func() {
				class = createVMClass(ctx, f, newNestedVirtualizationVMClass(f, v1alpha3.CPUTypeDiscovery, true))
			})

			// Only the features the discovery found end up required; an undiscovered one is left to
			// libvirt and the node the VirtualMachine lands on, which the spec cannot predict.
			provided := providedNestedVirtualizationFeatures(class)
			if len(provided) == 0 {
				Skip(fmt.Sprintf("no node of VirtualMachineClass %s provides vmx or svm, so the applied domain is up to libvirt", class.Name))
			}

			var vm *v1alpha2.VirtualMachine
			By("Start a VirtualMachine of the class", func() {
				vm = startVMOfClass(ctx, f, class.Name)
			})

			By("Verify the applied domain requires the discovered nested virtualization features", func() {
				cpu := readDomainCPU(ctx, f, vm)
				for _, feature := range provided {
					expectCPUFeaturePolicy(cpu, feature, cpuFeaturePolicyRequire)
				}
			})

			By("Verify the guest sees the discovered nested virtualization CPU flags", func() {
				flags := guestCPUFlags(f, vm)
				for _, feature := range provided {
					Expect(flags).To(ContainElement(feature),
						"the guest of VirtualMachine %s/%s should see the %q CPU flag", vm.Namespace, vm.Name, feature)
				}
			})
		})
	})
})

// newNestedVirtualizationVMClass names the cluster-scoped class after the namespace of the spec, so
// parallel specs never share one.
func newNestedVirtualizationVMClass(f *framework.Framework, cpuType v1alpha3.CPUType, nestedVirtualizationEnabled bool) *v1alpha3.VirtualMachineClass {
	name := fmt.Sprintf("%s-%s", f.Namespace().Name, strings.ToLower(string(cpuType)))
	if nestedVirtualizationEnabled {
		name = fmt.Sprintf("%s-nested", name)
	}

	class := &v1alpha3.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha3.SchemeGroupVersion.String(),
			Kind:       v1alpha3.VirtualMachineClassKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha3.VirtualMachineClassSpec{
			CPU: v1alpha3.CPU{
				Type:                       cpuType,
				EnableNestedVirtualization: ptr.To(nestedVirtualizationEnabled),
			},
		},
	}

	if cpuType == v1alpha3.CPUTypeDiscovery {
		class.Spec.CPU.Discovery = &v1alpha3.CpuDiscovery{
			NodeSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "node-role.kubernetes.io/control-plane",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
				},
			},
		}
	}

	return class
}

// createVMClass returns the ready class as the v1alpha2 object the rest of the suite works with: it
// is built as v1alpha3 while the observer and the status read below go through the v1alpha2 client;
// both address the same cluster-scoped resource.
func createVMClass(ctx context.Context, f *framework.Framework, class *v1alpha3.VirtualMachineClass) *v1alpha2.VirtualMachineClass {
	GinkgoHelper()

	classObs := startVMClassObserver(ctx, f, class.Name)
	Expect(f.CreateWithDeferredDeletion(ctx, class)).To(Succeed())
	Expect(classObs.WaitFor(vmclassobs.BeReady(), framework.MiddleTimeout)).To(Succeed())

	ready, err := f.VirtClient().VirtualMachineClasses().Get(ctx, class.Name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	return ready
}

func providedNestedVirtualizationFeatures(class *v1alpha2.VirtualMachineClass) []string {
	provided := make([]string, 0, len(nestedVirtualizationCPUFeatures))
	for _, feature := range nestedVirtualizationCPUFeatures {
		if slices.Contains(class.Status.CpuFeatures.Enabled, feature) {
			provided = append(provided, feature)
		}
	}
	return provided
}

// startVMOfClass boots the custom guest image: it has no cloud-init, the guest agent and the root
// SSH key are baked in, so no provisioning is needed.
func startVMOfClass(ctx context.Context, f *framework.Framework, className string) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	vd := object.NewVDFromCVI("vd", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
	vm := object.NewMinimalVM("", f.Namespace().Name,
		vmbuilder.WithName("vm"),
		vmbuilder.WithVirtualMachineClass(className),
		vmbuilder.WithDisks(vd),
	)

	Expect(f.CreateWithDeferredDeletion(ctx, vd, vm)).To(Succeed())

	vmObs := vmobs.StartObserver(ctx, f, vm)
	vmObs.Never(vmobs.BeFailed())
	Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
	eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)

	return vm
}

// domainSpec is the slice of the libvirt domain specification the specs assert on. `vlctl domain`
// marshals the KubeVirt domain structure, which carries json tags on no field, so the keys of its
// json output are the Go field names.
type domainSpec struct {
	CPU domainCPU
}

type domainCPU struct {
	Mode     string
	Model    string
	Features []domainCPUFeature
}

type domainCPUFeature struct {
	Name   string
	Policy string
}

// readDomainCPU reads the domain from the virt-launcher pod of the VirtualMachine. The domain
// appears a moment after the VirtualMachine reports Running, so the read is retried.
func readDomainCPU(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) domainCPU {
	GinkgoHelper()

	var cpu domainCPU
	Eventually(func() error {
		pod := getActiveVirtLauncherPod(ctx, f, vm.Name, vm.Namespace)
		if pod == nil {
			return fmt.Errorf("VirtualMachine %s/%s has no running virt-launcher pod", vm.Namespace, vm.Name)
		}

		cmd := f.Clients.Kubectl().RawCommandContext(ctx,
			fmt.Sprintf("exec %s --namespace %s -- vlctl domain -o json", pod.Name, pod.Namespace),
			kubectl.MediumTimeout,
		)
		if cmd.Error() != nil {
			return fmt.Errorf("read the domain of pod %s: %w: %s", pod.Name, cmd.Error(), cmd.StdErr())
		}

		var spec domainSpec
		if err := json.Unmarshal(cmd.StdOutBytes(), &spec); err != nil {
			return fmt.Errorf("unmarshal the domain of pod %s: %w", pod.Name, err)
		}

		cpu = spec.CPU
		return nil
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed(),
		"the domain of VirtualMachine %s/%s should be readable", vm.Namespace, vm.Name)

	return cpu
}

func expectCPUFeaturePolicy(cpu domainCPU, feature, policy string) {
	GinkgoHelper()

	for _, f := range cpu.Features {
		if f.Name == feature {
			Expect(f.Policy).To(Equal(policy),
				"the applied domain should carry the %q CPU feature with the %q policy, its CPU is %+v", feature, policy, cpu)
			return
		}
	}

	Fail(fmt.Sprintf("the applied domain carries no %q CPU feature, its CPU is %+v", feature, cpu))
}

// The command deliberately contains no single quotes (d8 wraps the guest command in '...').
const guestCPUFlagsCommand = `grep -m1 ^flags /proc/cpuinfo`

func guestCPUFlags(f *framework.Framework, vm *v1alpha2.VirtualMachine) []string {
	GinkgoHelper()

	out, err := f.SSHCommand(vm.Name, vm.Namespace, guestCPUFlagsCommand, framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred(),
		"the guest of VirtualMachine %s/%s should report its CPU flags", vm.Namespace, vm.Name)

	_, flags, found := strings.Cut(out, ":")
	Expect(found).To(BeTrue(), "unexpected /proc/cpuinfo flags line: %q", out)

	return strings.Fields(flags)
}
