/*
Copyright 2025 Flant JSC

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmclassobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmclass"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// startVMClassObserver starts a VirtualMachineClass observer by name. The test
// builds the class as v1alpha3 while the observer watches through the v1alpha2
// client; both address the same cluster-scoped resource.
func startVMClassObserver(ctx context.Context, f *framework.Framework, name string) vmclassobs.Observer {
	GinkgoHelper()
	return vmclassobs.StartObserver(ctx, f, &v1alpha2.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	})
}

// sizingPolicyMatchedReason reports the SizingPolicyMatched condition carries
// the given reason.
func sizingPolicyMatchedReason(reason vmcondition.SizingPolicyMatchedReason) vmobs.Predicate {
	return func(m *v1alpha2.VirtualMachine) (bool, error) {
		for _, c := range m.Status.Conditions {
			if c.Type == vmcondition.TypeSizingPolicyMatched.String() {
				return c.Reason == reason.String(), nil
			}
		}
		return false, nil
	}
}

var _ = Describe("SizingPolicy", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		t   *sizingPolicyTest
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("sizing-policy")
		f.Before()
		DeferCleanup(f.After)
		t = newSizingPolicyTest(f)

		ctx = context.Background()
	})

	It("should start VM normally with existing VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-vmclass", f.Namespace().Name)
		t.GenerateSizingPolicyResources(vmClassName, vmClassName)

		classObs := startVMClassObserver(ctx, f, t.VMClass.Name)
		err := f.CreateWithDeferredDeletion(ctx, t.VMClass)
		Expect(err).NotTo(HaveOccurred())
		err = classObs.WaitFor(vmclassobs.BeReady(), framework.ShortTimeout)
		Expect(err).NotTo(HaveOccurred())
		err = f.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		vmObs := vmobs.StartObserver(ctx, f, t.VM)
		vmObs.Never(vmobs.BeFailed())
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Validating VM by VMClass")
		t.ValidateVirtualMachineByClass(t.VMClass, t.VM)
	})

	It("should start VM after creating VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-existing-vmclass", f.Namespace().Name)
		t.GenerateSizingPolicyResources(vmClassName, vmClassName)

		err := f.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
		Expect(err).NotTo(HaveOccurred())
		vmObs := vmobs.StartObserver(ctx, f, t.VM)

		By("Waiting for SizingPolicyMatched condition reason to be VirtualMachineClassNotExists")
		err = vmObs.WaitFor(
			sizingPolicyMatchedReason(vmcondition.ReasonVirtualMachineClassNotFound),
			framework.LongTimeout,
		)
		Expect(err).NotTo(HaveOccurred())

		By("Creating VMClass")
		err = f.CreateWithDeferredDeletion(ctx, t.VMClass)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM to be ready")
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Validating VM by VMClass")
		t.ValidateVirtualMachineByClass(t.VMClass, t.VM)
	})

	It("should start VM after changing VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-actual-vmclass", f.Namespace().Name)
		vmClassNameInVM := fmt.Sprintf("%s-fake-vmclass", f.Namespace().Name)
		t.GenerateSizingPolicyResources(vmClassName, vmClassNameInVM)

		err := f.CreateWithDeferredDeletion(ctx, t.VMClass, t.VD, t.VM)
		Expect(err).NotTo(HaveOccurred())
		vmObs := vmobs.StartObserver(ctx, f, t.VM)

		By("Waiting for SizingPolicyMatched condition reason to be VirtualMachineClassNotExists")
		err = vmObs.WaitFor(
			sizingPolicyMatchedReason(vmcondition.ReasonVirtualMachineClassNotFound),
			framework.LongTimeout,
		)
		Expect(err).NotTo(HaveOccurred())

		By("Changing VMClass")
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/virtualMachineClassName",
			"value": vmClassName,
		}})
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Patch(ctx, t.VM, client.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM to be ready")
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Validating VM by VMClass")
		t.ValidateVirtualMachineByClass(t.VMClass, t.VM)
	})
})

type sizingPolicyTest struct {
	Framework *framework.Framework

	VM      *v1alpha2.VirtualMachine
	VD      *v1alpha2.VirtualDisk
	VMClass *v1alpha3.VirtualMachineClass
}

func newSizingPolicyTest(f *framework.Framework) *sizingPolicyTest {
	return &sizingPolicyTest{
		Framework: f,
	}
}

func (t *sizingPolicyTest) GenerateSizingPolicyResources(vmClassName, vmClassNameInVM string) {
	t.VD = object.NewVDFromCVI("vd", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
	)

	// The custom image has no cloud-init; the guest agent is baked in,
	// so no provisioning is configured. The sizing policy below is written
	// around the custom-image sizing (its memory minimum is that size), so the
	// VM validated against it stays as small as every other VM in the suites.
	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: t.VD.Name,
		}),
		vmbuilder.WithVirtualMachineClass(vmClassNameInVM),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
	)

	t.VMClass = &v1alpha3.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha3.SchemeGroupVersion.String(),
			Kind:       v1alpha3.VirtualMachineClassKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: vmClassName,
		},
		Spec: v1alpha3.VirtualMachineClassSpec{
			NodeSelector: v1alpha3.NodeSelector{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      "node.deckhouse.io/group",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"worker"},
					},
				},
			},
			CPU: v1alpha3.CPU{
				Type: v1alpha3.CPUTypeDiscovery,
				Discovery: &v1alpha3.CpuDiscovery{
					NodeSelector: metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "node-role.kubernetes.io/control-plane",
								Operator: metav1.LabelSelectorOpDoesNotExist,
							},
						},
					},
				},
			},
			SizingPolicies: []v1alpha3.SizingPolicy{
				{
					Cores: &v1alpha3.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha3.SizingPolicyMemory{
						MemoryMinMax: v1alpha3.MemoryMinMax{
							Min: ptr.To(resource.MustParse(object.CustomImageVMMemory)),
							Max: ptr.To(resource.MustParse("128Mi")),
						},
						Step: ptr.To(resource.MustParse(object.CustomImageVMMemory)),
					},
					CoreFractions: []v1alpha3.CoreFractionValue{
						"5%",
						"10%",
						"20%",
						"50%",
						"100%",
					},
					DedicatedCores: []bool{false},
				},
			},
		},
	}
}

func (t *sizingPolicyTest) ValidateVirtualMachineByClass(virtualMachineClass *v1alpha3.VirtualMachineClass, virtualMachine *v1alpha2.VirtualMachine) {
	var sizingPolicy v1alpha3.SizingPolicy
	for _, p := range virtualMachineClass.Spec.SizingPolicies {
		if virtualMachine.Spec.CPU.Cores >= p.Cores.Min && virtualMachine.Spec.CPU.Cores <= p.Cores.Max {
			sizingPolicy = *p.DeepCopy()
			break
		}
	}

	checkMinMemory := virtualMachine.Spec.Memory.Size.Value() >= sizingPolicy.Memory.Min.Value()
	checkMaxMemory := virtualMachine.Spec.Memory.Size.Value() <= sizingPolicy.Memory.Max.Value()
	checkMemory := checkMinMemory && checkMaxMemory
	Expect(checkMemory).To(BeTrue(), fmt.Errorf("memory size outside of possible interval '%v - %v': %v", sizingPolicy.Memory.Min, sizingPolicy.Memory.Max, virtualMachine.Spec.Memory.Size))

	checkCoreFraction := slices.Contains(sizingPolicy.CoreFractions, v1alpha3.CoreFractionValue(virtualMachine.Spec.CPU.CoreFraction))
	Expect(checkCoreFraction).To(BeTrue(), fmt.Errorf("sizing policy core fraction list does not contain value from spec: %s\n%v", virtualMachine.Spec.CPU.CoreFraction, sizingPolicy.CoreFractions))
}
