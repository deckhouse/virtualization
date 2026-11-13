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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

var _ = Describe("PeerMigrationLockError", func() {
	const ns = "app-x"

	newVM := func(name, disk string, anns map[string]string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Annotations: anns},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: disk}},
			},
		}
	}
	// movingKVVM is the KubeVirt VM of a VM that moves by the operation.
	movingKVVM := func(name, op string) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Annotations: map[string]string{annotations.AnnPeerMigrationVMOP: op}}}
	}
	move := &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: ns},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeMigrate,
			VirtualMachine: "web",
			Migrate:        &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: "cluster-b"},
		},
	}

	It("names the peer of a VM that moves there", func() {
		vm := newVM("web", "web-root", nil)
		c, err := newClient(vm, move, movingKVVM("web", "move"))
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(MatchError(ContainSubstring(`migrated to peer cluster "cluster-b"`)))
	})

	It("locks a VM whose operation is not found", func() {
		vm := newVM("web", "web-root", nil)
		c, err := newClient(vm, movingKVVM("web", "gone"))
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(MatchError(ContainSubstring("migrated to a peer cluster")))
	})

	// The KubeVirt VM may not be seen: the condition the VM controller set from the mark stands for it.
	It("locks a VM whose status shows it moves while its KubeVirt VM is not seen", func() {
		vm := newVM("web", "web-root", nil)
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeMovedToPeer.String(), Status: metav1.ConditionTrue}}
		c, err := newClient(vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(MatchError(ContainSubstring("migrated to a peer cluster")))
	})

	It("leaves alone a VM whose KubeVirt VM carries no mark", func() {
		vm := newVM("web", "web-root", map[string]string{"virtualization.deckhouse.io/moved-to-peer": "move"})
		c, err := newClient(vm, move, &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns}})
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(Succeed())
	})

	It("locks a VM whose status shows it is received from a peer", func() {
		vm := newVM("web", "web-root", nil)
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeReceivingFromPeer.String(), Status: metav1.ConditionTrue}}
		c, err := newClient(vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(MatchError(ContainSubstring("migrated from a peer cluster")))
	})

	// The status shows the receive a few seconds after the request created the VM: the reference to the
	// request locks the VM in between.
	It("locks a VM created by a request before its status shows the receive", func() {
		vm := newVM("web", "web-root", nil)
		vm.OwnerReferences = []metav1.OwnerReference{{Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind, Name: "web", UID: "uid-1"}}
		c, err := newClient(vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(MatchError(ContainSubstring("migrated from a peer cluster")))
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", ns)).To(MatchError(ContainSubstring("migrated from a peer cluster")))
	})

	It("leaves a VM owned by something else alone", func() {
		vm := newVM("web", "web-root", nil)
		vm.OwnerReferences = []metav1.OwnerReference{{Kind: "VirtualMachinePool", Name: "pool", UID: "uid-1"}}
		c, err := newClient(vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(Succeed())
	})

	It("leaves a VM whose receive is over alone", func() {
		vm := newVM("web", "web-root", nil)
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeReceivingFromPeer.String(), Status: metav1.ConditionFalse}}
		c, err := newClient(vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(Succeed())
	})

	It("leaves a free VM alone", func() {
		vm := newVM("web", "web-root", nil)
		c, err := newClient(vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockError(ctx(), c, vm)).To(Succeed())
	})

	It("locks an attached disk removed from the current spec", func() {
		moving := newVM("web", "next-root", nil)
		moving.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: "web-root", Attached: true}}
		c, err := newClient(moving, move, movingKVVM("web", "move"))
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", ns)).To(MatchError(ContainSubstring("cluster-b")))
	})

	DescribeTable("locks the disks from the running spec before attachment status is available", func(annotation string) {
		moving := newVM("web", "next-root", nil)
		kvvm := movingKVVM("web", "move")
		spec, err := json.Marshal(newVM("web", "web-root", nil).Spec)
		Expect(err).NotTo(HaveOccurred())

		kvvm.Annotations[annotation] = string(spec)
		c, err := newClient(moving, move, kvvm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", ns)).To(MatchError(ContainSubstring("cluster-b")))
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "next-root", ns)).To(MatchError(ContainSubstring("cluster-b")))
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "unrelated", ns)).To(Succeed())
	}, Entry("current annotation", annotations.AnnVMLastAppliedSpec), Entry("legacy annotation", annotations.AnnVMLastAppliedSpecLegacy))

	It("does not lock an old disk of a VM that is not migrating", func() {
		vm := newVM("db", "next-root", nil)
		kvvm := movingKVVM("db", "")
		kvvm.Annotations[annotations.AnnVMLastAppliedSpec] = `{"blockDeviceRefs":[{"kind":"VirtualDisk","name":"web-root"}]}`
		c, err := newClient(vm, kvvm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", ns)).To(Succeed())
	})

	It("does not allow a disk change when the running spec cannot be read", func() {
		kvvm := movingKVVM("web", "move")
		kvvm.Annotations[annotations.AnnVMLastAppliedSpec] = "invalid json"
		c, err := newClient(newVM("web", "next-root", nil), move, kvvm)
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", ns)).NotTo(Succeed())
	})

	It("locks only the disks of a moving VM", func() {
		moving := newVM("web", "web-root", nil)
		free := newVM("db", "db-root", nil)
		c, err := newClient(moving, free, move, movingKVVM("web", "move"))
		Expect(err).NotTo(HaveOccurred())
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", ns)).To(MatchError(ContainSubstring("cluster-b")))
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "db-root", ns)).To(Succeed())
		Expect(PeerMigrationLockErrorForDisk(ctx(), c, "web-root", "other")).To(Succeed())
	})
})

func ctx() context.Context { return context.Background() }

// newClient is a fake client without testutil: testutil imports this package through the indexer.
func newClient(objs ...client.Object) (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		return nil, err
	}

	if err := virtv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(), nil
}
