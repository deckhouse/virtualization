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

package release

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// migrationWindow spans the Evict VirtualMachineOperation the upgrade created
// for a virtual machine; the VMOP outlives the migration, unlike the migration
// state of a recreated instance.
type migrationWindow struct {
	start time.Time
	end   time.Time
}

func findMigrationWindow(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) migrationWindow {
	GinkgoHelper()

	vmops, err := f.Clients.VirtClient().VirtualMachineOperations(vm.Namespace).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	var newest *v1alpha2.VirtualMachineOperation
	for i := range vmops.Items {
		vmop := &vmops.Items[i]
		if vmop.Spec.VirtualMachine != vm.Name || vmop.Spec.Type != v1alpha2.VMOPTypeEvict {
			continue
		}
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted),
			"Evict VirtualMachineOperation %s/%s of virtual machine %s must be Completed", vmop.Namespace, vmop.Name, vm.Name)
		if newest == nil || vmop.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = vmop
		}
	}
	Expect(newest).NotTo(BeNil(),
		"virtual machine %s/%s has no Evict VirtualMachineOperation: the upgrade was expected to migrate it (set %s=false for an upgrade that does not)",
		vm.Namespace, vm.Name, migratesVMsEnv)

	window := migrationWindow{start: newest.CreationTimestamp.Time}
	for _, c := range newest.Status.Conditions {
		if c.Type == vmopcondition.TypeCompleted.String() {
			window.end = c.LastTransitionTime.Time
			break
		}
	}
	Expect(window.end.IsZero()).To(BeFalse(),
		"Evict VirtualMachineOperation %s/%s has no Completed condition, so the migration end is unknown", newest.Namespace, newest.Name)
	Expect(window.end.Before(window.start)).To(BeFalse(), "the migration end must not precede its start")

	return window
}
