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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("LifeCycleHandler move to a peer cluster", func() {
	DescribeTable("shows the source VM",
		func(migrationUID string, completed bool, phase, expected v1alpha2.MachinePhase) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default", Annotations: map[string]string{annotations.AnnVMMovedToPeer: "move"}},
				Status:     v1alpha2.VirtualMachineStatus{Phase: phase},
			}
			vmim := &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Name: "vmop-move", Namespace: "default", UID: "move-uid"}}
			kvvmi := &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Status: virtv1.VirtualMachineInstanceStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
					MigrationUID: "earlier-uid", Completed: completed,
				}},
			}
			if migrationUID != "" {
				kvvmi.Status.MigrationState.MigrationUID = "move-uid"
			}
			fakeClient, err := testutil.NewFakeClientWithObjects(vm, vmim, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(NewLifeCycleHandler(fakeClient, nil).syncMovePhase(context.Background(), vm, kvvmi)).To(Succeed())
			Expect(vm.Status.Phase).To(Equal(expected))
		},
		Entry("running after an earlier migration completed", "", true, v1alpha2.MachineRunning, v1alpha2.MachineRunning),
		Entry("migrating while the KVVM already reports Running", "move", false, v1alpha2.MachineRunning, v1alpha2.MachineMigrating),
		Entry("stopped once the move completed", "move", true, v1alpha2.MachineRunning, v1alpha2.MachineStopped),
	)
})
