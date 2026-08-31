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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("PodHandler", func() {
	const (
		namespace  = "default"
		vmName     = "vm"
		sourcePod  = "virt-launcher-vm-source"
		targetPod  = "virt-launcher-vm-target"
		sourceNode = "node-a"
		targetNode = "node-b"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	// The source pod is always older than the pod the migration creates for the target.
	born := metav1.NewTime(time.Now().Add(-time.Hour))

	newPod := func(name, node string, uid types.UID, created metav1.Time) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				UID:               uid,
				CreationTimestamp: created,
				Labels:            map[string]string{virtv1.VirtualMachineNameLabel: vmName},
				Finalizers:        []string{v1alpha2.FinalizerPodProtection},
			},
			Spec:   corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	newKVVMI := func(nodeName string, migration *virtv1.VirtualMachineInstanceMigrationState) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
			Status: virtv1.VirtualMachineInstanceStatus{
				Phase:    virtv1.Running,
				NodeName: nodeName,
				ActivePods: map[types.UID]string{
					"source-uid": sourceNode,
					"target-uid": targetNode,
				},
				MigrationState: migration,
			},
		}
	}

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
			Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning},
		}
	}

	hasProtection := func(fakeClient client.Client, name string) bool {
		pod := &corev1.Pod{}
		err := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pod)
		Expect(err).NotTo(HaveOccurred())
		return controllerutil.ContainsFinalizer(pod, v1alpha2.FinalizerPodProtection)
	}

	// A hung source pod never reaches a terminal phase, so its protection can only be
	// released by noticing that the migration moved the instance away from it.
	It("releases the source pod left behind by a completed migration", func() {
		kvvmi := newKVVMI(targetNode, &virtv1.VirtualMachineInstanceMigrationState{
			SourcePod: sourcePod,
			TargetPod: targetPod,
			Completed: true,
		})
		fakeClient, _, vmState := setupEnvironment(newVM(),
			newEmptyKVVM(vmName, namespace),
			kvvmi,
			newPod(sourcePod, sourceNode, "source-uid", born),
			newPod(targetPod, targetNode, "target-uid", metav1.NewTime(born.Add(time.Minute))),
		)

		_, err := NewPodHandler(fakeClient).Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasProtection(fakeClient, sourcePod)).To(BeFalse())
		Expect(hasProtection(fakeClient, targetPod)).To(BeTrue())
	})

	It("keeps both pods protected while the migration is in flight", func() {
		kvvmi := newKVVMI(sourceNode, &virtv1.VirtualMachineInstanceMigrationState{
			SourcePod: sourcePod,
			TargetPod: targetPod,
		})
		fakeClient, _, vmState := setupEnvironment(newVM(),
			newEmptyKVVM(vmName, namespace),
			kvvmi,
			newPod(sourcePod, sourceNode, "source-uid", born),
			newPod(targetPod, targetNode, "target-uid", metav1.NewTime(born.Add(time.Minute))),
		)

		_, err := NewPodHandler(fakeClient).Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasProtection(fakeClient, sourcePod)).To(BeTrue())
		Expect(hasProtection(fakeClient, targetPod)).To(BeTrue())
	})

	It("releases the target pod of a failed migration", func() {
		kvvmi := newKVVMI(sourceNode, &virtv1.VirtualMachineInstanceMigrationState{
			SourcePod: sourcePod,
			TargetPod: targetPod,
			Failed:    true,
		})
		fakeClient, _, vmState := setupEnvironment(newVM(),
			newEmptyKVVM(vmName, namespace),
			kvvmi,
			newPod(sourcePod, sourceNode, "source-uid", born),
			newPod(targetPod, targetNode, "target-uid", metav1.NewTime(born.Add(time.Minute))),
		)

		_, err := NewPodHandler(fakeClient).Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasProtection(fakeClient, sourcePod)).To(BeTrue())
		Expect(hasProtection(fakeClient, targetPod)).To(BeFalse())
	})

	// KubeVirt creates the target pod before it records the migration on the instance,
	// so for a short while the target pod is backed by nothing but its creation time.
	It("keeps a target pod protected before the migration is recorded", func() {
		fakeClient, _, vmState := setupEnvironment(newVM(),
			newEmptyKVVM(vmName, namespace),
			newKVVMI(sourceNode, nil),
			newPod(sourcePod, sourceNode, "source-uid", born),
			newPod(targetPod, targetNode, "target-uid", metav1.NewTime(born.Add(time.Minute))),
		)

		_, err := NewPodHandler(fakeClient).Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasProtection(fakeClient, sourcePod)).To(BeTrue())
		Expect(hasProtection(fakeClient, targetPod)).To(BeTrue())
	})

	It("keeps the only pod protected when no migration happened", func() {
		fakeClient, _, vmState := setupEnvironment(newVM(),
			newEmptyKVVM(vmName, namespace),
			newKVVMI(sourceNode, nil),
			newPod(sourcePod, sourceNode, "source-uid", born),
		)

		_, err := NewPodHandler(fakeClient).Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		Expect(hasProtection(fakeClient, sourcePod)).To(BeTrue())
	})
})
