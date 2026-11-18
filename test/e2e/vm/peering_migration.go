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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	peertest "github.com/deckhouse/virtualization/test/e2e/internal/peering"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("PeeringMigration", label.EE(), Label(label.SIGCompute, precheck.PrecheckPeering), func() {
	var h *peertest.PeerMigration

	BeforeEach(func() { h = peertest.NewPeerMigration() })

	It("uses the preallocated target address and preserves the guest and disk", func() {
		h.Start()
		address := h.ReserveAddress(h.Target)
		h.MigrateAndVerify(address)
	})

	It("returns immediately after Completed and preserves the guest and disk", func() {
		h.Start()
		outboundAddress := h.ReserveAddress(h.Target)
		returnAddress := h.ReserveAddress(h.Source)
		h.RenewUntil(h.Source, returnAddress.Status.Address)
		outbound := h.Operation(h.Source, outboundAddress)
		h.Create(h.Source, outbound, false)
		h.WaitPhase(outbound, v1alpha2.VMOPPhaseCompleted, framework.MaxTimeout)
		// Waiting for guest SSH or MachineRunning here would hide a premature Completed.
		returning := h.Operation(h.Target, returnAddress)
		h.Create(h.Target, returning, false)
		h.WaitPhase(returning, v1alpha2.VMOPPhaseCompleted, framework.MaxTimeout)
		h.AssertMigrationCleanup(h.Target, h.Source)
		h.AssertGuest(h.Source, returnAddress)
		h.AssertTerminal(returning, v1alpha2.VMOPPhaseCompleted)
		h.AssertTerminal(outbound, v1alpha2.VMOPPhaseCompleted)
	})

	// Regression for https://flant.kaiten.ru/74595165: a returned VM's new instance
	// was deleted immediately after creation, leaving Start in progress.
	It("restarts and starts after a round trip with AlwaysOnUnlessStoppedManually", func() {
		By("Move an AlwaysOn VM to the peer and back without rebooting the guest")

		h.VM.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy
		h.Start()

		outboundAddress := h.ReserveAddress(h.Target)
		returnAddress := h.ReserveAddress(h.Source)
		h.RenewUntil(h.Source, returnAddress.Status.Address)
		outbound := h.Operation(h.Source, outboundAddress)
		h.Create(h.Source, outbound, false)
		h.WaitPhase(outbound, v1alpha2.VMOPPhaseCompleted, framework.MaxTimeout)
		util.UntilObjectsDeleted(h.Context, framework.MiddleTimeout, h.VM, h.Disk)
		h.AssertGuest(h.Target, outboundAddress)
		returning := h.Operation(h.Target, returnAddress)
		h.Create(h.Target, returning, false)
		h.WaitPhase(returning, v1alpha2.VMOPPhaseCompleted, framework.MaxTimeout)
		h.AssertGuest(h.Source, returnAddress)
		h.AssertTerminal(outbound, v1alpha2.VMOPPhaseCompleted)

		By("Change the returned VM policy to AlwaysOnUnlessStoppedManually")

		returned := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: h.VM.Name, Namespace: h.Source.Namespace().Name}}

		Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(returned), returned)).To(Succeed())
		running := meta.FindStatusCondition(returned.Status.Conditions, string(vmcondition.TypeRunning))
		Expect(running).NotTo(BeNil(), "the returned VM must report its running state")
		previousRunningTime := running.LastTransitionTime.Time
		observer := vmobs.StartObserver(h.Context, h.Source, returned)

		Eventually(func(g Gomega) {
			g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(returned), returned)).To(Succeed())
			before := returned.DeepCopy()
			returned.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually
			returned.Spec.Disruptions = &v1alpha2.Disruptions{RestartApprovalMode: v1alpha2.Automatic}
			g.Expect(h.Source.GenericClient().Patch(h.Context, returned, client.MergeFrom(before))).To(Succeed())
		}).WithTimeout(framework.ShortTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "update the returned VM's run policy")

		Expect(observer.WaitFor(vmobs.BeRunningAfterRebootAfter(previousRunningTime), framework.LongTimeout)).To(Succeed(), "automatic restart must apply the new run policy")
		eventually.SSHReadyAsRoot(h.Source, returned, framework.LongTimeout)

		for _, action := range []v1alpha2.VMOPType{v1alpha2.VMOPTypeRestart, v1alpha2.VMOPTypeStop} {
			By("Run " + string(action) + " on the returned VM")

			bootBefore, diskBefore := peertest.PeerGuestIdentity(h.Source, returned)
			op := vmopbuilder.New(
				vmopbuilder.WithGenerateName("vmop-peer-power-"),
				vmopbuilder.WithNamespace(returned.Namespace),
				vmopbuilder.WithVirtualMachine(returned.Name),
				vmopbuilder.WithType(action),
			)
			Expect(util.CreateVMOPRetryingStaleActiveDenial(h.Context, h.Source, op)).To(Succeed(), "create %s for %s/%s", action, returned.Namespace, returned.Name)
			h.WaitPhase(op, v1alpha2.VMOPPhaseCompleted, framework.LongTimeout)

			if action == v1alpha2.VMOPTypeStop {
				util.UntilObjectPhase(h.Context, string(v1alpha2.MachineStopped), framework.MiddleTimeout, returned)
				By("Start the stopped VM and wait for the operation to complete")

				start := util.StartVirtualMachine(h.Context, h.Source, returned)
				h.WaitPhase(start, v1alpha2.VMOPPhaseCompleted, framework.LongTimeout)
			}

			util.UntilObjectPhase(h.Context, string(v1alpha2.MachineRunning), framework.LongTimeout, returned)
			eventually.SSHReadyAsRoot(h.Source, returned, framework.LongTimeout)
			bootAfter, diskAfter := peertest.PeerGuestIdentity(h.Source, returned)
			Expect(bootAfter).NotTo(Equal(bootBefore), "%s must create a new guest boot", action)
			Expect(diskAfter).To(Equal(diskBefore), "%s must preserve the received disk", action)
			Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(returned), returned)).To(Succeed())
			Expect(returned.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnUnlessStoppedManually), "power operations must preserve the user's run policy")
			Expect(returned.Status.IPAddress).To(Equal(returnAddress.Status.Address), "the VM must keep its reserved address after %s", action)
		}
	})

	DescribeTable("cancels and immediately allows a new move", Label(framework.LabelTolerateFailedMigrations), func(stage string) {
		h.Start()
		saved := h.Runtime()
		address := h.ReserveAddress(h.Target)
		op := h.Operation(h.Source, address)
		h.Create(h.Source, op, true)

		switch stage {
		case "preparing":
			h.WaitPreparing(op)
		case "copying":
			h.WaitCopying(op)
		}

		h.CancelAndVerify(op, address, saved)
		h.MigrateAndVerify(address)
	}, Entry("deleted immediately", "immediate"), Entry("deleted on WaitingForPeer", "preparing"), Entry("deleted during copying", "copying"))

	It("keeps the quota cause on every rejection and succeeds once the quota is raised", Label(framework.LabelTolerateFailedMigrations), func() {
		h.Start()
		saved := h.Runtime()
		address := h.ReserveAddress(h.Target)
		quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "peer-pods", Namespace: h.Target.Namespace().Name}, Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("0")}}}
		Expect(h.Target.CreateWithDeferredDeletion(h.Context, quota)).To(Succeed())

		for range 2 {
			op := h.Operation(h.Source, address)
			h.Create(h.Source, op, true)
			h.WaitPhase(op, v1alpha2.VMOPPhaseFailed, framework.MiddleTimeout)
			completed := meta.FindStatusCondition(op.Status.Conditions, string(vmopcondition.TypeCompleted))
			Expect(completed).NotTo(BeNil())
			Expect(completed.Message).To(ContainSubstring("quota"))
			Expect(completed.Message).To(ContainSubstring(quota.Name))
			h.AssertRollbackCleanup(address)
			h.AssertSource(saved)
			h.AssertTerminal(op, v1alpha2.VMOPPhaseFailed)
		}

		Expect(h.Target.GenericClient().Delete(h.Context, quota)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(h.Target.GenericClient().Get(h.Context, client.ObjectKeyFromObject(quota), &corev1.ResourceQuota{}))
		}).WithTimeout(framework.ShortTimeout).Should(BeTrue())
		h.MigrateAndVerify(address)
	})

	DescribeTable("keeps the source when the receiving workload is deleted", Label(framework.LabelTolerateFailedMigrations), func(kind string) {
		h.Start()
		saved := h.Runtime()
		address := h.ReserveAddress(h.Target)
		target := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: h.VM.Name, Namespace: h.Target.Namespace().Name}}
		h.Target.ExpectFailure(target)
		op := h.Operation(h.Source, address)
		h.Create(h.Source, op, true)

		if kind == "VirtualMachine" {
			h.WaitCopying(op)
			Expect(h.Target.GenericClient().Delete(h.Context, target)).To(Succeed())
		} else {
			h.WaitPreparing(op)

			var pod *corev1.Pod

			Eventually(func(g Gomega) {
				pod = getActiveVirtLauncherPod(h.Context, h.Target, h.VM.Name, h.Target.Namespace().Name)
				g.Expect(pod).NotTo(BeNil())
			}).WithTimeout(framework.MiddleTimeout).WithPolling(100 * time.Millisecond).Should(Succeed())
			Expect(h.Target.GenericClient().Delete(h.Context, pod)).To(Succeed())
		}

		h.WaitPhase(op, v1alpha2.VMOPPhaseFailed, framework.MiddleTimeout)
		completed := meta.FindStatusCondition(op.Status.Conditions, string(vmopcondition.TypeCompleted))
		Expect(completed).NotTo(BeNil())

		if kind == "VirtualMachine" {
			Expect(completed.Message).To(ContainSubstring(h.VM.Name))
			Expect(completed.Message).To(ContainSubstring("deleted"))
		}

		h.AssertRollbackCleanup(address)
		h.AssertSource(saved)
		h.MigrateAndVerify(address)
	}, Entry("the receiving VirtualMachine", "VirtualMachine"), Entry("the receiving launcher pod", "Pod"))

	DescribeTable("retries after a rejected receive once its dependency is fixed", Label(framework.LabelTolerateFailedMigrations), func(conflict string) {
		var (
			foreign *v1alpha2.VirtualMachine
			secret  *corev1.Secret
		)
		if conflict == "Secret" {
			secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "peer-cloud-init", Namespace: h.Source.Namespace().Name}, Type: v1alpha2.SecretTypeCloudInit, StringData: map[string]string{"userData": "#cloud-config\n"}}
			Expect(h.Source.CreateWithDeferredDeletion(h.Context, secret)).To(Succeed())
			h.VM.Spec.Provisioning = &v1alpha2.Provisioning{Type: v1alpha2.ProvisioningTypeUserDataRef, UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: secret.Name}}
		}

		h.Start()
		saved := h.Runtime()

		address := h.ReserveAddress(h.Target)
		if conflict == "VirtualMachine" {
			foreign = h.VM.DeepCopy()
			foreign.ObjectMeta = metav1.ObjectMeta{Name: h.VM.Name, Namespace: h.Target.Namespace().Name}
			foreign.Status = v1alpha2.VirtualMachineStatus{}
			foreign.Spec.RunPolicy = v1alpha2.AlwaysOffPolicy
			Expect(h.Target.CreateWithDeferredDeletion(h.Context, foreign)).To(Succeed())
		}

		op := h.Operation(h.Source, address)
		h.Create(h.Source, op, true)
		h.WaitPhase(op, v1alpha2.VMOPPhaseFailed, framework.MiddleTimeout)
		completed := meta.FindStatusCondition(op.Status.Conditions, string(vmopcondition.TypeCompleted))
		Expect(completed).NotTo(BeNil())

		if foreign != nil {
			Expect(completed.Message).To(ContainSubstring(foreign.Name))

			current := &v1alpha2.VirtualMachine{}
			Expect(h.Target.GenericClient().Get(h.Context, client.ObjectKeyFromObject(foreign), current)).To(Succeed())
			Expect(current.UID).To(Equal(foreign.UID))
			Expect(h.Target.GenericClient().Delete(h.Context, foreign)).To(Succeed())
		} else {
			Expect(completed.Message).To(ContainSubstring(secret.Name))
			copy := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secret.Name, Namespace: h.Target.Namespace().Name}, Type: secret.Type, StringData: map[string]string{"userData": "#cloud-config\n"}}
			Expect(h.Target.CreateWithDeferredDeletion(h.Context, copy)).To(Succeed())
		}

		h.AssertSource(saved)
		h.AssertRollbackCleanup(address)
		h.MigrateAndVerify(address)
	}, Entry("a VM name conflict", "VirtualMachine"), Entry("a missing cloud-init Secret", "Secret"))

	DescribeTable("rejects invalid operations before starting migration", func(stopped bool) {
		if stopped {
			h.VM.Spec.RunPolicy = v1alpha2.AlwaysOffPolicy
			Expect(h.Source.CreateWithDeferredDeletion(h.Context, h.Disk, h.VM)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(h.VM), h.VM)).To(Succeed())
				g.Expect(h.VM.Status.Phase).To(Equal(v1alpha2.MachineStopped))
			}).WithTimeout(framework.LongTimeout).Should(Succeed())
		} else {
			h.Start()
		}

		address := h.ReserveAddress(h.Target)

		op := h.Operation(h.Source, address)
		if !stopped {
			op.Spec.Migrate.ProjectPeerName = "no-such-e2e-peer"
		}

		err := h.Source.GenericClient().Create(h.Context, op)
		Expect(err).To(HaveOccurred())

		if stopped {
			Expect(err.Error()).To(ContainSubstring("running"))
		} else {
			Expect(err.Error()).To(ContainSubstring("paired"))
		}
	}, Entry("an unknown peer", false), Entry("a stopped VM", true))

	It("rejects conflicting changes during copying and completes the first operation", func() {
		h.Start()
		address := h.ReserveAddress(h.Target)
		h.RenewAddress(h.Source)
		op := h.Operation(h.Source, address)
		h.Create(h.Source, op, false)
		h.WaitCopying(op)
		Expect(h.Source.GenericClient().Create(h.Context, h.Operation(h.Source, address))).To(
			MatchError(And(ContainSubstring("Previously created operation"), ContainSubstring(op.Name))),
			"the active VMOP rejects a second operation",
		)
		err := h.Source.GenericClient().Delete(h.Context, h.VM)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(op.Name), "the denial names the VMOP to cancel")

		old := h.Disk.DeepCopy()
		resized := h.Disk.DeepCopy()
		resized.Spec.PersistentVolumeClaim.Size = ptr.To(resource.MustParse("3Gi"))
		Expect(h.Source.GenericClient().Patch(h.Context, resized, client.MergeFrom(old))).To(
			MatchError(And(ContainSubstring("migrat"), ContainSubstring(op.Name))),
			"disk resize is rejected because this VMOP locks the virtual machine",
		)
		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "during-move", Namespace: h.VM.Namespace}, Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: h.VM.Name}}
		diskSnapshot := &v1alpha2.VirtualDiskSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "during-move", Namespace: h.VM.Namespace}, Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: h.Disk.Name}}
		Expect(h.Source.GenericClient().Create(h.Context, vmSnapshot)).To(
			MatchError(And(ContainSubstring("migrat"), ContainSubstring(op.Name))),
			"a virtual machine snapshot is rejected because this VMOP locks the virtual machine",
		)
		Expect(h.Source.GenericClient().Create(h.Context, diskSnapshot)).To(
			MatchError(And(ContainSubstring("migrat"), ContainSubstring(op.Name))),
			"a disk snapshot is rejected because this VMOP locks the virtual machine",
		)
		h.WaitPhase(op, v1alpha2.VMOPPhaseCompleted, framework.MaxTimeout)
		h.AssertMigrationCleanup(h.Source, h.Target)
		h.AssertGuest(h.Target, address)
		h.AssertTerminal(op, v1alpha2.VMOPPhaseCompleted)
	})
	It("admits a request to receive from a peer cluster through the restricted access policy", func() {
		// The peering module presents a peer cluster as the service account of its synchronization controller
		// in the group of the capability it acts for: peering:peer:<peer>:system:serviceaccount:<ns>:<sa>,
		// with the group peering:peer:<peer>:cap:virtualization. The policy closing the internal API group
		// must let it write the requests to receive and nothing else; system:masters keeps RBAC out of the
		// way, so a failure here is the policy and not a missing binding.
		dst, ctx, peer := h.Target, h.Context, h.Peer
		peerUser := "peering:peer:" + peer + ":system:serviceaccount:" + framework.ModuleNamespace + ":kubevirt-internal-virtualization-synchronization-controller"
		asPeer, err := framework.GetClients().ImpersonatedClient(peerUser,
			"peering:peer:"+peer+":cap:virtualization", "system:authenticated", "system:masters")
		Expect(err).NotTo(HaveOccurred())

		request := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Namespace: dst.Namespace().Name, Name: "policy-check"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"policy-check"},"spec":{}}`)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
				IPAddress: "10.66.10.1",
			},
		}
		Expect(asPeer.Create(ctx, request, client.DryRunAll)).To(Succeed(), "the peer may create a request to receive")

		// An administrator cancels an incoming move by deleting the request; the policy leaves that to RBAC.
		Expect(asPeer.Create(ctx, request)).To(Succeed())
		dst.DeferDelete(request)
		Expect(dst.GenericClient().Delete(ctx, request)).To(Succeed(), "an administrator may delete a request to receive")

		// The smallest spec the API server accepts. An object it rejects by its schema or a kubevirt
		// webhook never reaches the policy, so the check would not test the policy at all.
		kvvm := &unstructured.Unstructured{Object: map[string]any{
			"spec": map[string]any{
				"runStrategy": "Halted",
				"template":    map[string]any{"spec": map[string]any{"domain": map[string]any{"devices": map[string]any{}}}},
			},
		}}
		kvvm.SetGroupVersionKind(schema.GroupVersionKind{Group: "internal.virtualization.deckhouse.io", Version: "v1", Kind: "InternalVirtualizationVirtualMachine"})
		kvvm.SetNamespace(dst.Namespace().Name)
		kvvm.SetName("policy-check")
		err = asPeer.Create(ctx, kvvm, client.DryRunAll)
		Expect(err).To(HaveOccurred(), "the peer may not write other internal resources")
		Expect(err.Error()).To(ContainSubstring("virtualization-restricted-access-policy"))
	})
})
