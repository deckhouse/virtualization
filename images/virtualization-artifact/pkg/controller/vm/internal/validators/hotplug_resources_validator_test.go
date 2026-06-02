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

package validators_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("HotplugResourcesValidator", func() {
	type testCase struct {
		oldVM     *v1alpha2.VirtualMachine
		newVM     *v1alpha2.VirtualMachine
		objects   []client.Object
		wantError string
	}

	DescribeTable("ValidateUpdate",
		func(tc testCase) {
			validator := validators.NewHotplugResourcesValidator(newFakeClientForHotplugValidator(tc.objects...))
			_, err := validator.ValidateUpdate(context.Background(), tc.oldVM, tc.newVM)

			if tc.wantError == "" {
				Expect(err).NotTo(HaveOccurred())
				return
			}

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.wantError))
		},
		Entry("should skip validation when cpu and memory are unchanged", testCase{
			oldVM:     newVMForHotplugValidation(4, "50%", "8Gi"),
			newVM:     newVMForHotplugValidation(4, "50%", "8Gi"),
			wantError: "",
		}),
		Entry("should fail when hotplug cores exceed allowed maximum", testCase{
			oldVM:     newVMForHotplugValidation(64, "100%", "64Gi"),
			newVM:     newVMForHotplugValidation(129, "100%", "64Gi"),
			wantError: "hotplug CPU cores should not exceed 128",
		}),
		Entry("should fail when hotplug memory exceeds allowed maximum", testCase{
			oldVM:     newVMForHotplugValidation(16, "100%", "128Gi"),
			newVM:     newVMForHotplugValidation(16, "100%", "257Gi"),
			wantError: "hotplug memory should not exceed 256Gi",
		}),
		Entry("should fail when quota is insufficient during migration", testCase{
			oldVM: newVMForHotplugValidation(2, "100%", "8Gi"),
			newVM: newVMForHotplugValidation(4, "100%", "8Gi"),
			objects: []client.Object{
				newResourceQuota(
					"default",
					resource.MustParse("6"),
					resource.MustParse("8Gi"),
					resource.MustParse("3"),
					resource.MustParse("4Gi"),
				),
			},
			wantError: "insufficient project quota",
		}),
		Entry("should pass when quota is sufficient", testCase{
			oldVM: newVMForHotplugValidation(2, "100%", "8Gi"),
			newVM: newVMForHotplugValidation(4, "100%", "8Gi"),
			objects: []client.Object{
				newResourceQuota(
					"default",
					resource.MustParse("10"),
					resource.MustParse("32Gi"),
					resource.MustParse("2"),
					resource.MustParse("8Gi"),
				),
			},
			wantError: "",
		}),
	)
})

func newVMForHotplugValidation(cores int, coreFraction, memory string) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm",
			Namespace: "default",
		},
		Spec: v1alpha2.VirtualMachineSpec{
			CPU: v1alpha2.CPUSpec{
				Cores:        cores,
				CoreFraction: coreFraction,
			},
			Memory: v1alpha2.MemorySpec{
				Size: resource.MustParse(memory),
			},
		},
	}
}

func newResourceQuota(namespace string, cpuHard, memoryHard, cpuUsed, memoryUsed resource.Quantity) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "project-quota",
			Namespace: namespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    cpuHard,
				corev1.ResourceRequestsMemory: memoryHard,
			},
		},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    cpuHard,
				corev1.ResourceRequestsMemory: memoryHard,
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    cpuUsed,
				corev1.ResourceRequestsMemory: memoryUsed,
			},
		},
	}
}

func newFakeClientForHotplugValidator(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}
