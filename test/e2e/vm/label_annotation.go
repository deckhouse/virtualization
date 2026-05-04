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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	metadataSpecialKey   = "specialKey"
	metadataSpecialValue = "specialValue"
)

var _ = Describe("VirtualMachineLabelAndAnnotation", Label(precheck.NoPrecheck), func() {
	var f *framework.Framework

	BeforeEach(func() {
		f = framework.NewFramework("vm-label-annotation")
		DeferCleanup(f.After)
		f.Before()
	})

	It("propagates labels and annotations from VM to active pod", func() {
		ctx := context.Background()

		By("Environment preparation")
		vm := object.NewMinimalVM("vm-label-annotation-", f.Namespace().Name,
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: object.PrecreatedCVIUbuntuISO,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

		By(fmt.Sprintf("Adding label %q=%q to VM", metadataSpecialKey, metadataSpecialValue))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			if current.Labels == nil {
				current.Labels = make(map[string]string)
			}
			current.Labels[metadataSpecialKey] = metadataSpecialValue
		})

		By("Checking that label is present on VM and active pod")
		expectLabelState(ctx, f, vm, true)

		By(fmt.Sprintf("Removing label %q from VM", metadataSpecialKey))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			delete(current.Labels, metadataSpecialKey)
		})

		By("Checking that label is absent on VM and active pod")
		expectLabelState(ctx, f, vm, false)

		By(fmt.Sprintf("Adding annotation %q=%q to VM", metadataSpecialKey, metadataSpecialValue))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			if current.Annotations == nil {
				current.Annotations = make(map[string]string)
			}
			current.Annotations[metadataSpecialKey] = metadataSpecialValue
		})

		By("Checking that annotation is present on VM and active pod")
		expectAnnotationState(ctx, f, vm, true)

		By(fmt.Sprintf("Removing annotation %q from VM", metadataSpecialKey))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			delete(current.Annotations, metadataSpecialKey)
		})

		By("Checking that annotation is absent on VM and active pod")
		expectAnnotationState(ctx, f, vm, false)
	})
})

func updateVirtualMachineMetadata(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, mutate func(*v1alpha2.VirtualMachine)) {
	GinkgoHelper()

	Eventually(func() error {
		var currentVM v1alpha2.VirtualMachine
		err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), &currentVM)
		if err != nil {
			return err
		}

		mutate(&currentVM)
		err = f.GenericClient().Update(ctx, &currentVM)
		return err
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func expectLabelState(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, isPresent bool) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		currentVM, activePod, err := getVirtualMachineAndActivePod(ctx, f, vm)
		g.Expect(err).NotTo(HaveOccurred())

		if isPresent {
			g.Expect(currentVM.Labels).To(HaveKeyWithValue(metadataSpecialKey, metadataSpecialValue))
			g.Expect(activePod.Labels).To(HaveKeyWithValue(metadataSpecialKey, metadataSpecialValue))
			return
		}

		g.Expect(currentVM.Labels).NotTo(HaveKey(metadataSpecialKey))
		g.Expect(activePod.Labels).NotTo(HaveKey(metadataSpecialKey))
	}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func expectAnnotationState(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, isPresent bool) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		currentVM, activePod, err := getVirtualMachineAndActivePod(ctx, f, vm)
		g.Expect(err).NotTo(HaveOccurred())

		if isPresent {
			g.Expect(currentVM.Annotations).To(HaveKeyWithValue(metadataSpecialKey, metadataSpecialValue))
			g.Expect(activePod.Annotations).To(HaveKeyWithValue(metadataSpecialKey, metadataSpecialValue))
			return
		}

		g.Expect(currentVM.Annotations).NotTo(HaveKey(metadataSpecialKey))
		g.Expect(activePod.Annotations).NotTo(HaveKey(metadataSpecialKey))
	}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func getVirtualMachineAndActivePod(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) (*v1alpha2.VirtualMachine, *corev1.Pod, error) {
	currentVM, err := f.VirtClient().VirtualMachines(vm.Namespace).Get(ctx, vm.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	activePodName, err := getActiveVirtualMachinePodName(currentVM)
	if err != nil {
		return nil, nil, err
	}

	activePod, err := f.KubeClient().CoreV1().Pods(vm.Namespace).Get(ctx, activePodName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	return currentVM, activePod, nil
}

func getActiveVirtualMachinePodName(vm *v1alpha2.VirtualMachine) (string, error) {
	for _, pod := range vm.Status.VirtualMachinePods {
		if pod.Active {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("active pod was not found for vm %s/%s", vm.Namespace, vm.Name)
}
