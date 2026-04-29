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

package migration

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	migrationstore "github.com/deckhouse/virtualization-controller/pkg/migration/store"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("Migration VM Running Transition Time", func() {
	const namespace = "vm-running-transition-time"

	It("should set Running condition transition time from KVVMI creation timestamp and mark migration completed", func() {
		creationTime := metav1.NewTime(time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC))
		wrongTime := metav1.NewTime(time.Date(2026, 4, 29, 10, 25, 6, 0, time.UTC))

		vm := newRunningVM(namespace, "vm", wrongTime)
		kvvmi := newKVVMI(namespace, "vm", creationTime)

		fakeClient, err := testutil.NewFakeClientWithObjects(vm, kvvmi)
		Expect(err).NotTo(HaveOccurred())

		m, err := newVMRunningTransitionTime(fakeClient, testutil.NewNoOpLogger())
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Migrate(context.Background())).To(Succeed())

		updatedVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, updatedVM)).To(Succeed())
		cond := findRunningCondition(updatedVM.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		Expect(cond.LastTransitionTime.Equal(&creationTime)).To(BeTrue())

		marker := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: migrationstore.ConfigMapName, Namespace: migrationstore.Namespace}, marker)).To(Succeed())
		Expect(marker.Data).To(HaveKey(vmRunningTransitionTimeMigrationName))
	})

	It("should not run after completion marker is set", func() {
		creationTime := metav1.NewTime(time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC))
		wrongTime := metav1.NewTime(time.Date(2026, 4, 29, 10, 25, 6, 0, time.UTC))
		completedMarker := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: migrationstore.ConfigMapName, Namespace: migrationstore.Namespace},
			Data:       map[string]string{vmRunningTransitionTimeMigrationName: "2026-04-29T10:30:00Z"},
		}

		vm := newRunningVM(namespace, "vm", wrongTime)
		kvvmi := newKVVMI(namespace, "vm", creationTime)

		fakeClient, err := testutil.NewFakeClientWithObjects(vm, kvvmi, completedMarker)
		Expect(err).NotTo(HaveOccurred())

		m, err := newVMRunningTransitionTime(fakeClient, testutil.NewNoOpLogger())
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Migrate(context.Background())).To(Succeed())

		updatedVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, updatedVM)).To(Succeed())
		cond := findRunningCondition(updatedVM.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		Expect(cond.LastTransitionTime.Equal(&wrongTime)).To(BeTrue())
	})

	It("should preserve other migration records in shared ConfigMap", func() {
		creationTime := metav1.NewTime(time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC))
		wrongTime := metav1.NewTime(time.Date(2026, 4, 29, 10, 25, 6, 0, time.UTC))
		marker := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: migrationstore.ConfigMapName, Namespace: migrationstore.Namespace},
			Data:       map[string]string{"other-migration": "2026-04-29T10:30:00Z"},
		}

		vm := newRunningVM(namespace, "vm", wrongTime)
		kvvmi := newKVVMI(namespace, "vm", creationTime)

		fakeClient, err := testutil.NewFakeClientWithObjects(vm, kvvmi, marker)
		Expect(err).NotTo(HaveOccurred())

		m, err := newVMRunningTransitionTime(fakeClient, testutil.NewNoOpLogger())
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Migrate(context.Background())).To(Succeed())

		updatedMarker := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: migrationstore.ConfigMapName, Namespace: migrationstore.Namespace}, updatedMarker)).To(Succeed())
		Expect(updatedMarker.Data).To(HaveKeyWithValue("other-migration", "2026-04-29T10:30:00Z"))
		Expect(updatedMarker.Data).To(HaveKey(vmRunningTransitionTimeMigrationName))
	})
})

func newRunningVM(namespace, name string, transitionTime metav1.Time) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Status: v1alpha2.VirtualMachineStatus{
			Conditions: []metav1.Condition{
				{
					Type:               vmcondition.TypeRunning.String(),
					Status:             metav1.ConditionTrue,
					Reason:             vmcondition.ReasonVirtualMachineRunning.String(),
					ObservedGeneration: 1,
					LastTransitionTime: transitionTime,
				},
			},
		},
	}
}

func newKVVMI(namespace, name string, creationTime metav1.Time) *virtv1.VirtualMachineInstance {
	return &virtv1.VirtualMachineInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.GroupVersion.String(),
			Kind:       "VirtualMachineInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: creationTime,
		},
	}
}

func findRunningCondition(conditions []metav1.Condition) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == vmcondition.TypeRunning.String() {
			return &conditions[i]
		}
	}
	return nil
}
