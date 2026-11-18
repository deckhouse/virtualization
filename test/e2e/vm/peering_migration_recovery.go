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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	peertest "github.com/deckhouse/virtualization/test/e2e/internal/peering"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

// A self-peer shares one synchronization controller between both sides of the move.
var _ = Describe("PeeringMigrationRecovery", label.EE(), Serial, Label(label.SIGCompute, precheck.PrecheckPeering, precheck.PrecheckPeeringRecovery, framework.LabelTolerateFailedMigrations), func() {
	var h *peertest.PeerMigration

	BeforeEach(func(ctx SpecContext) {
		Expect(peertest.CheckMigrationIsolation(ctx, framework.GetClients().RewriteClient(), "")).To(Succeed())

		h = peertest.NewPeerMigration()
	})
	DescribeTable("recovers from a synchronization controller restart while copying", func(action string) {
		h.Start()
		saved := h.Runtime()

		address := h.ReserveAddress(h.Target)
		if action == "finish" {
			h.RenewAddress(h.Source)
		}

		op := h.Operation(h.Source, address)
		h.Create(h.Source, op, true)
		h.WaitCopying(op)

		Expect(peertest.CheckMigrationIsolation(h.Context, h.Source.RewriteClient(), h.VM.Name,
			h.Source.Namespace().Name, h.Target.Namespace().Name)).To(Succeed())

		deployment, err := h.Source.KubeClient().AppsV1().Deployments(framework.ModuleNamespace).Get(h.Context, "virt-synchronization-controller", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
		Expect(err).NotTo(HaveOccurred())
		pods, err := h.Source.KubeClient().CoreV1().Pods(framework.ModuleNamespace).List(h.Context, metav1.ListOptions{LabelSelector: selector.String()})
		Expect(err).NotTo(HaveOccurred())

		live := []corev1.Pod{}

		for _, pod := range pods.Items {
			if pod.DeletionTimestamp.IsZero() {
				live = append(live, pod)
			}
		}

		Expect(live).To(HaveLen(1), "identify the synchronization process being restarted")
		old := live[0]
		Expect(h.Source.KubeClient().CoreV1().Pods(framework.ModuleNamespace).Delete(h.Context, old.Name, metav1.DeleteOptions{GracePeriodSeconds: ptr.To[int64](1), Preconditions: &metav1.Preconditions{UID: ptr.To(old.UID)}})).To(Succeed())

		if action != "finish" {
			h.CancelAndVerify(op, address, saved)
		}

		Eventually(func(g Gomega) {
			replacement, err := h.Source.KubeClient().CoreV1().Pods(framework.ModuleNamespace).List(h.Context, metav1.ListOptions{LabelSelector: selector.String()})
			g.Expect(err).NotTo(HaveOccurred())

			ready := false

			for _, pod := range replacement.Items {
				if pod.UID == old.UID || !pod.DeletionTimestamp.IsZero() {
					continue
				}

				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						ready = true
					}
				}
			}

			g.Expect(ready).To(BeTrue())
		}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "replacement synchronization controller is ready")

		switch action {
		case "finish":
			Eventually(func(g Gomega) {
				g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(op), op)).To(Succeed())
				g.Expect(op.Status.Phase).To(BeElementOf(v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseFailed))
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "the restarted synchronization cannot leave the operation stuck")

			if op.Status.Phase == v1alpha2.VMOPPhaseCompleted {
				h.AssertMigrationCleanup(h.Source, h.Target)
				h.AssertGuest(h.Target, address)
				h.AssertTerminal(op, v1alpha2.VMOPPhaseCompleted)
			} else {
				h.AssertRollbackCleanup(address)
				h.AssertSource(saved)
				h.AssertTerminal(op, v1alpha2.VMOPPhaseFailed)
				h.MigrateAndVerify(address)
			}
		case "cancel":
			h.MigrateAndVerify(address)
		case "delete":
			Expect(h.Source.GenericClient().Delete(h.Context, h.VM)).To(Succeed(), "VM deletion is allowed after cancellation")
			Eventually(func(g Gomega) {
				key := types.NamespacedName{Namespace: h.VM.Namespace, Name: h.VM.Name}
				g.Expect(apierrors.IsNotFound(h.Source.GenericClient().Get(h.Context, key, &v1alpha2.VirtualMachine{}))).To(BeTrue())
				g.Expect(apierrors.IsNotFound(h.Source.RewriteClient().Get(h.Context, key.Name, &rewrite.VirtualMachineInstance{}, rewrite.InNamespace(key.Namespace)))).To(BeTrue())

				remaining := &rewrite.VirtualMachineInstanceMigrationList{}
				g.Expect(h.Source.RewriteClient().List(h.Context, remaining, rewrite.InNamespace(h.VM.Namespace))).To(Succeed())
				g.Expect(remaining.Items).To(BeEmpty(), "no source VMIM remains on its finalizer")
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
			h.AssertRollbackCleanup(address)
		}
	}, Entry("waits for a definite outcome", "finish"), Entry("cancels the move and retries", "cancel"), Entry("deletes the VM and operation without leaving a VMIM", "delete"))
})
