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
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("SizingPolicy", func() {
	var t *sizingPolicyTest
	f := framework.NewFramework("sizing-policy")

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)
		t = newSizingPolicyTest(f)
	})

	It("should start VM normally with existing VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-vmclass", f.Namespace().Name)
		t.GenerateSizingPolicyResources(vmClassName, vmClassName)

		err := f.CreateWithDeferredDeletion(context.Background(), t.VMClass)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(string(v1alpha2.ClassPhaseReady), framework.ShortTimeout, t.VMClass)
		err = f.CreateWithDeferredDeletion(context.Background(), t.VD, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		util.UntilVMAgentReady(client.ObjectKeyFromObject(t.VM), framework.LongTimeout)

		By("Validating VM by VMClass")
		t.ValidateVirtualMachineByClass(t.VMClass, t.VM)
	})

	It("should start VM after creating VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-existing-vmclass", f.Namespace().Name)
		t.GenerateSizingPolicyResources(vmClassName, vmClassName)

		err := f.CreateWithDeferredDeletion(context.Background(), t.VD, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for SizingPolicyMatched condition reason to be VirtualMachineClassNotExists")
		util.UntilConditionReason(vmcondition.TypeSizingPolicyMatched.String(), vmcondition.ReasonVirtualMachineClassNotFound.String(), framework.LongTimeout, t.VM)

		By("Creating VMClass")
		err = f.CreateWithDeferredDeletion(context.Background(), t.VMClass)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM to be ready")
		util.UntilVMAgentReady(client.ObjectKeyFromObject(t.VM), framework.LongTimeout)

		By("Validating VM by VMClass")
		t.ValidateVirtualMachineByClass(t.VMClass, t.VM)
	})

	It("should start VM after changing VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-actual-vmclass", f.Namespace().Name)
		vmClassNameInVM := fmt.Sprintf("%s-fake-vmclass", f.Namespace().Name)
		t.GenerateSizingPolicyResources(vmClassName, vmClassNameInVM)

		err := f.CreateWithDeferredDeletion(context.Background(), t.VMClass, t.VD, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for SizingPolicyMatched condition reason to be VirtualMachineClassNotExists")
		util.UntilConditionReason(vmcondition.TypeSizingPolicyMatched.String(), vmcondition.ReasonVirtualMachineClassNotFound.String(), framework.LongTimeout, t.VM)

		By("Changing VMClass")
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/virtualMachineClassName",
			"value": vmClassName,
		}})
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Patch(context.Background(), t.VM, client.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM to be ready")
		util.UntilVMAgentReady(client.ObjectKeyFromObject(t.VM), framework.LongTimeout)

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
	t.VD = vdbuilder.New(
		vdbuilder.WithName("vd"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLAlpineBIOS,
		}),
	)

	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: t.VD.Name,
		}),
		vmbuilder.WithVirtualMachineClass(vmClassNameInVM),
		vmbuilder.WithCPU(1, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse("1Gi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
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
							Min: ptr.To(resource.MustParse("1Gi")),
							Max: ptr.To(resource.MustParse("8Gi")),
						},
						Step: ptr.To(resource.MustParse("512Mi")),
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
