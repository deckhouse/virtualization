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
	"k8s.io/client-go/util/retry"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	podobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/pod"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	metadataSpecialKey   = "specialKey"
	metadataSpecialValue = "specialValue"
)

var _ = Describe("VirtualMachineLabelAndAnnotation", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var f *framework.Framework

	BeforeEach(func() {
		f = framework.NewFramework("vm-label-and-annotation")
		DeferCleanup(f.After)
		f.Before()
	})

	It("propagates labels and annotations from VM to active pod", func() {
		ctx := context.Background()

		By("Environment preparation")
		// The custom ISO boots straight to a kernel, which is all this
		// test needs: it only waits for the Running phase and never talks to
		// the guest. The custom image has no cloud-init, so no provisioning.
		vm := object.NewMinimalVM("vm-label-annotation-", f.Namespace().Name,
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: object.PrecreatedCVICustomISO,
			}),
		)

		err := f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		// The VM uses generateName, so its name is known only after creation;
		// an observer armed earlier would watch an empty name and never match.
		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())

		err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		// The active pod exists from now on; its observer is armed once, before
		// any metadata mutation, so every propagation event below is captured.
		_, activePod, err := util.GetVirtualMachineAndActivePod(ctx, f, vm)
		Expect(err).NotTo(HaveOccurred())
		podObs := podobs.StartObserver(ctx, f, activePod)

		By(fmt.Sprintf("Adding label %q=%q to VM", metadataSpecialKey, metadataSpecialValue))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			if current.Labels == nil {
				current.Labels = make(map[string]string)
			}
			current.Labels[metadataSpecialKey] = metadataSpecialValue
		})

		By("Checking that label is present on VM and active pod")
		expectLabelState(vmObs, podObs, true)

		By(fmt.Sprintf("Removing label %q from VM", metadataSpecialKey))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			delete(current.Labels, metadataSpecialKey)
		})

		By("Checking that label is absent on VM and active pod")
		expectLabelState(vmObs, podObs, false)

		By(fmt.Sprintf("Adding annotation %q=%q to VM", metadataSpecialKey, metadataSpecialValue))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			if current.Annotations == nil {
				current.Annotations = make(map[string]string)
			}
			current.Annotations[metadataSpecialKey] = metadataSpecialValue
		})

		By("Checking that annotation is present on VM and active pod")
		expectAnnotationState(vmObs, podObs, true)

		By(fmt.Sprintf("Removing annotation %q from VM", metadataSpecialKey))
		updateVirtualMachineMetadata(ctx, f, vm, func(current *v1alpha2.VirtualMachine) {
			delete(current.Annotations, metadataSpecialKey)
		})

		By("Checking that annotation is absent on VM and active pod")
		expectAnnotationState(vmObs, podObs, false)
	})
})

// updateVirtualMachineMetadata applies mutate to a fresh copy of the VM,
// retrying on optimistic-lock conflicts with other controllers.
func updateVirtualMachineMetadata(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, mutate func(*v1alpha2.VirtualMachine)) {
	GinkgoHelper()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var currentVM v1alpha2.VirtualMachine
		err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), &currentVM)
		if err != nil {
			return err
		}

		mutate(&currentVM)
		return f.GenericClient().Update(ctx, &currentVM)
	})
	Expect(err).NotTo(HaveOccurred())
}

// expectLabelState waits, via the pre-armed VM and pod observers, until the
// special label is present on (or absent from) both the VirtualMachine and
// its active pod.
func expectLabelState(vmObs vmobs.Observer, podObs podobs.Observer, isPresent bool) {
	GinkgoHelper()

	err := vmObs.WaitFor(vmLabelState(isPresent), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	err = podObs.WaitFor(podLabelState(isPresent), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

// vmLabelState reports the special label is present on (or absent from) the
// VirtualMachine.
func vmLabelState(isPresent bool) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return metadataKeyMatches(vm.Labels, isPresent), nil
	}
}

// podLabelState reports the special label is present on (or absent from) the
// pod.
func podLabelState(isPresent bool) podobs.Predicate {
	return func(pod *corev1.Pod) (bool, error) {
		return metadataKeyMatches(pod.Labels, isPresent), nil
	}
}

// expectAnnotationState waits, via the pre-armed VM and pod observers, until
// the special annotation is present on (or absent from) both the
// VirtualMachine and its active pod.
func expectAnnotationState(vmObs vmobs.Observer, podObs podobs.Observer, isPresent bool) {
	GinkgoHelper()

	err := vmObs.WaitFor(vmAnnotationState(isPresent), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	err = podObs.WaitFor(podAnnotationState(isPresent), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

// vmAnnotationState reports the special annotation is present on (or absent
// from) the VirtualMachine.
func vmAnnotationState(isPresent bool) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return metadataKeyMatches(vm.Annotations, isPresent), nil
	}
}

// podAnnotationState reports the special annotation is present on (or absent
// from) the pod.
func podAnnotationState(isPresent bool) podobs.Predicate {
	return func(pod *corev1.Pod) (bool, error) {
		return metadataKeyMatches(pod.Annotations, isPresent), nil
	}
}

func metadataKeyMatches(meta map[string]string, isPresent bool) bool {
	value, ok := meta[metadataSpecialKey]
	if isPresent {
		return ok && value == metadataSpecialValue
	}
	return !ok
}
