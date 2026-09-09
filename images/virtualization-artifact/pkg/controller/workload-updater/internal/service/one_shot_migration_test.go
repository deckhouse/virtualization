/*
Copyright 2025 Flant JSC

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

package service

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestOnceShotMigrationService", func() {
	const (
		vmName      = "vm"
		vmNamespace = "default"
		prefix      = "vmop-prefix-"
	)

	newVM := func() *v1alpha2.VirtualMachine {
		return vmbuilder.NewEmpty(vmName, vmNamespace)
	}

	newKVVMI := func() *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
				Annotations: map[string]string{
					"key": "old-value",
				},
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineInstance",
				APIVersion: virtv1.GroupVersion.String(),
			},
		}
	}

	newRecorder := func() *eventrecord.EventRecorderLoggerMock {
		recorder := &eventrecord.EventRecorderLoggerMock{}
		recorder.EventfFunc = func(_ client.Object, _, _, _ string, _ ...interface{}) {}
		recorder.WithLoggingFunc = func(_ eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
			return recorder
		}
		return recorder
	}

	getVM := func(fakeClient client.Client) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{}
		err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: vmNamespace, Name: vmName}, vm)
		Expect(err).ToNot(HaveOccurred())
		return vm
	}

	listVMOPs := func(fakeClient client.Client) []v1alpha2.VirtualMachineOperation {
		vmops := v1alpha2.VirtualMachineOperationList{}
		err := fakeClient.List(context.Background(), &vmops)
		Expect(err).ToNot(HaveOccurred())
		return vmops.Items
	}

	finishVMOPs := func(fakeClient client.Client, phase v1alpha2.VMOPPhase) {
		for _, vmop := range listVMOPs(fakeClient) {
			if vmop.Status.Phase == "" {
				vmop.Status.Phase = phase
				Expect(fakeClient.Update(context.Background(), &vmop)).To(Succeed())
			}
		}
	}

	kvvmiAnnotation := func(fakeClient client.Client, key string) string {
		kvvmi := &virtv1.VirtualMachineInstance{}
		err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: vmNamespace, Name: vmName}, kvvmi)
		Expect(err).ToNot(HaveOccurred())
		return kvvmi.GetAnnotations()[key]
	}

	It("Retry 10 times expect one migration", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects(newVM(), newKVVMI())
		Expect(err).ToNot(HaveOccurred())

		oneShotMigration := NewOneShotMigrationService(fakeClient, newRecorder(), prefix)

		migrateCount := 0

		for i := 0; i < 10; i++ {
			migrate, err := oneShotMigration.OnceMigrate(testutil.ContextBackgroundWithNoOpLogger(), getVM(fakeClient), "key", "value")
			Expect(err).ToNot(HaveOccurred())
			if migrate {
				migrateCount++
			}
		}
		Expect(migrateCount).To(Equal(1))

		vmops := listVMOPs(fakeClient)
		Expect(vmops).To(HaveLen(1))
		Expect(vmops[0].Name).To(HavePrefix(prefix))
	})

	It("Marks the trigger handled only after the migration completes", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects(newVM(), newKVVMI())
		Expect(err).ToNot(HaveOccurred())

		oneShotMigration := NewOneShotMigrationService(fakeClient, newRecorder(), prefix)
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		migrate, err := oneShotMigration.OnceMigrate(ctx, getVM(fakeClient), "key", "value")
		Expect(err).ToNot(HaveOccurred())
		Expect(migrate).To(BeTrue())
		Expect(kvvmiAnnotation(fakeClient, "key")).To(Equal("old-value"))

		finishVMOPs(fakeClient, v1alpha2.VMOPPhaseCompleted)

		migrate, err = oneShotMigration.OnceMigrate(ctx, getVM(fakeClient), "key", "value")
		Expect(err).ToNot(HaveOccurred())
		Expect(migrate).To(BeFalse())
		Expect(kvvmiAnnotation(fakeClient, "key")).To(Equal("value"))
		Expect(listVMOPs(fakeClient)).To(HaveLen(1))
	})

	It("Retries a failed migration and gives up after the attempt limit", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects(newVM(), newKVVMI())
		Expect(err).ToNot(HaveOccurred())

		recorder := newRecorder()
		oneShotMigration := NewOneShotMigrationService(fakeClient, recorder, prefix)
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		for attempt := 1; attempt <= maxMigrationAttempts; attempt++ {
			migrate, err := oneShotMigration.OnceMigrate(ctx, getVM(fakeClient), "key", "value")
			Expect(err).ToNot(HaveOccurred())
			Expect(migrate).To(BeTrue(), "attempt %d must create a new migration", attempt)
			Expect(listVMOPs(fakeClient)).To(HaveLen(attempt))
			finishVMOPs(fakeClient, v1alpha2.VMOPPhaseFailed)
		}

		migrate, err := oneShotMigration.OnceMigrate(ctx, getVM(fakeClient), "key", "value")
		Expect(err).ToNot(HaveOccurred())
		Expect(migrate).To(BeFalse())
		Expect(listVMOPs(fakeClient)).To(HaveLen(maxMigrationAttempts))
		Expect(kvvmiAnnotation(fakeClient, "key")).To(Equal("value"))
		Expect(recorder.EventfCalls()).To(HaveLen(1))
		Expect(recorder.EventfCalls()[0].Reason).To(Equal(v1alpha2.ReasonWorkloadUpdateFailed))
	})

	It("Does not count failures of another trigger value", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects(newVM(), newKVVMI())
		Expect(err).ToNot(HaveOccurred())

		oneShotMigration := NewOneShotMigrationService(fakeClient, newRecorder(), prefix)
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		for attempt := 1; attempt <= maxMigrationAttempts; attempt++ {
			migrate, err := oneShotMigration.OnceMigrate(ctx, getVM(fakeClient), "key", "stale-value")
			Expect(err).ToNot(HaveOccurred())
			Expect(migrate).To(BeTrue())
			finishVMOPs(fakeClient, v1alpha2.VMOPPhaseFailed)
		}

		migrate, err := oneShotMigration.OnceMigrate(ctx, getVM(fakeClient), "key", "value")
		Expect(err).ToNot(HaveOccurred())
		Expect(migrate).To(BeTrue())
		Expect(listVMOPs(fakeClient)).To(HaveLen(maxMigrationAttempts + 1))
	})
})
