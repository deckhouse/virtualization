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
	f := framework.NewFramework("sizing-policy")

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)
	})

	It("should start VM normally with existing VMClass", func() {
		Expect(true).To(BeTrue())

		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-vmclass", f.Namespace().Name)
		vmClass, vd, vm := generateSizingPolicyResources(f.Namespace().Name, vmClassName, vmClassName)

		err := f.CreateWithDeferredDeletion(context.Background(), vmClass, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		util.UntilVMAgentReady(client.ObjectKeyFromObject(vm), framework.LongTimeout)
	})

	It("should start VM after creating VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-existing-vmclass", f.Namespace().Name)
		vmClass, vd, vm := generateSizingPolicyResources(f.Namespace().Name, vmClassName, vmClassName)

		err := f.CreateWithDeferredDeletion(context.Background(), vd, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for SizingPolicyMatched condition reason to be VirtualMachineClassNotExists")
		util.UntilConditionReason(vmcondition.TypeSizingPolicyMatched.String(), vmcondition.ReasonVirtualMachineClassNotExists.String(), framework.LongTimeout, vm)

		By("Creating VMClass")
		err = f.CreateWithDeferredDeletion(context.Background(), vmClass)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM to be ready")
		util.UntilVMAgentReady(client.ObjectKeyFromObject(vm), framework.LongTimeout)
	})

	It("should start VM after changing VMClass", func() {
		By("Environment preparation")
		vmClassName := fmt.Sprintf("%s-actual-vmclass", f.Namespace().Name)
		vmClassNameInVM := fmt.Sprintf("%s-fake-vmclass", f.Namespace().Name)
		vmClass, vd, vm := generateSizingPolicyResources(f.Namespace().Name, vmClassName, vmClassNameInVM)

		err := f.CreateWithDeferredDeletion(context.Background(), vmClass, vd, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for SizingPolicyMatched condition reason to be VirtualMachineClassNotExists")
		util.UntilConditionReason(vmcondition.TypeSizingPolicyMatched.String(), vmcondition.ReasonVirtualMachineClassNotExists.String(), framework.LongTimeout, vm)

		By("Changing VMClass")
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/virtualMachineClassName",
			"value": vmClassName,
		}})
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Patch(context.Background(), vm, client.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM to be ready")
		util.UntilVMAgentReady(client.ObjectKeyFromObject(vm), framework.LongTimeout)
	})
})

func generateSizingPolicyResources(namespace, vmClassName, vmClassNameInVM string) (vmClass *v1alpha3.VirtualMachineClass, vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine) {
	vd = vdbuilder.New(
		vdbuilder.WithName("vd"),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	vm = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: vd.Name,
		}),
		vmbuilder.WithVirtualMachineClass(vmClassNameInVM),
		vmbuilder.WithCPU(1, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse("1Gi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
	)

	vmClass = &v1alpha3.VirtualMachineClass{
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
							Min: resource.MustParse("1Gi"),
							Max: resource.MustParse("8Gi"),
						},
						Step: resource.MustParse("512Mi"),
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

	return
}
