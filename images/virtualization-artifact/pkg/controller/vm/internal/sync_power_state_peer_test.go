/*
Copyright 2024 Flant JSC

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

var _ = Describe("Peer power-state handoff", func() {
	const noReceiver intvirtv1alpha1.VirtualMachineReceiverPhase = ""
	DescribeTable("normalizes strategy only after ordinary power management resumes",
		func(phase virtv1.VirtualMachineInstancePhase, deleting, marked bool,
			receiverPhase intvirtv1alpha1.VirtualMachineReceiverPhase,
			initial, expected virtv1.VirtualMachineRunStrategy,
		) {
			ctx := testutil.ContextBackgroundWithNoOpLogger()
			key := types.NamespacedName{Namespace: "test", Name: "web"}
			vm, kvvm, vmi, pod := createObjectsForPowerstateTest(key)
			vm.UID = "vm-uid"
			vm.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually

			kvvm.Spec.RunStrategy = ptr.To(initial)
			if marked {
				kvvm.Annotations[annotations.AnnPeerMigrationVMOP] = "move"
			}

			objects := []client.Object{kvvm, pod}

			if phase != "" {
				vmi.Status.Phase = phase
				if deleting {
					vmi.DeletionTimestamp = ptr.To(metav1.Now())
					vmi.Finalizers = []string{"test.keep-vmi"}
				}

				objects = append(objects, vmi)
			}

			if receiverPhase != "" {
				objects = append(objects, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
					ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name, UID: "receiver-uid"},
					Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: receiverPhase, VirtualMachineUID: vm.UID},
				})
			}

			cl, _, vmState := setupEnvironment(vm, objects...)

			var recorder *eventrecord.EventRecorderLoggerMock

			recorder = &eventrecord.EventRecorderLoggerMock{
				EventfFunc:      func(client.Object, string, string, string, ...interface{}) {},
				WithLoggingFunc: func(eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
			}
			_, err := NewSyncPowerStateHandler(cl, recorder).Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			var actual virtv1.VirtualMachine
			Expect(cl.Get(ctx, key, &actual)).To(Succeed())
			Expect(actual.Spec.RunStrategy).To(Equal(ptr.To(expected)))
		},
		Entry("running VMI with Always", virtv1.Running, false, false, noReceiver, virtv1.RunStrategyAlways, virtv1.RunStrategyManual),
		Entry("running VMI with Halted", virtv1.Running, false, false, noReceiver, virtv1.RunStrategyHalted, virtv1.RunStrategyManual),
		Entry("failed VMI with Halted", virtv1.Failed, false, false, noReceiver, virtv1.RunStrategyHalted, virtv1.RunStrategyManual),
		Entry("missing VMI", virtv1.VirtualMachineInstancePhase(""), false, false, noReceiver, virtv1.RunStrategyHalted, virtv1.RunStrategyHalted),
		Entry("deleting VMI", virtv1.Running, true, false, noReceiver, virtv1.RunStrategyHalted, virtv1.RunStrategyHalted),
		Entry("source still owned by VMOP", virtv1.Running, false, true, noReceiver, virtv1.RunStrategyAlways, virtv1.RunStrategyAlways),
		Entry("completed receive", virtv1.Running, false, false, intvirtv1alpha1.ReceiverRunning, virtv1.RunStrategyAlways, virtv1.RunStrategyManual),
		Entry("unfinished receive", virtv1.Running, false, false, intvirtv1alpha1.ReceiverReady, virtv1.RunStrategyWaitAsReceiver, virtv1.RunStrategyWaitAsReceiver),
		Entry("failed receive", virtv1.Running, false, false, intvirtv1alpha1.ReceiverFailed, virtv1.RunStrategyWaitAsReceiver, virtv1.RunStrategyWaitAsReceiver),
	)
})
