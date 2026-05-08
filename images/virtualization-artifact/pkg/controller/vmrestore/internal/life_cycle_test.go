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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
)

var _ = Describe("LifeCycleHandler", func() {
	It("maps VirtualMachineMACAddressName by original network index", func() {
		vm := &v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork},
				},
			},
		}

		vmmac := &v1alpha2.VirtualMachineMACAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineMACAddressKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm-mac-secondary",
				Namespace: "default",
			},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
				Address:        "02:00:00:00:00:11",
				VirtualMachine: "vm",
			},
		}

		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualMachineSnapshotSpec{
				VirtualMachineName: "vm",
			},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				VirtualMachineSnapshotSecretName: "restorer-secret",
			},
		}

		vmRestore := &v1alpha2.VirtualMachineRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "restore",
				Namespace: "default",
				UID:       types.UID("restore-uid"),
			},
			Spec: v1alpha2.VirtualMachineRestoreSpec{
				VirtualMachineSnapshotName: "snapshot",
				RestoreMode:                v1alpha2.RestoreModeSafe,
			},
			Status: v1alpha2.VirtualMachineRestoreStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmrestorecondition.VirtualMachineSnapshotReadyToUseType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		restorerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "restorer-secret",
				Namespace: "default",
			},
		}

		fakeClient, err := testutil.NewFakeClientWithObjects(vmSnapshot, vmRestore, restorerSecret)
		Expect(err).NotTo(HaveOccurred())

		restorerMock := &RestorerMock{
			RestoreVirtualMachineFunc: func(context.Context, *corev1.Secret) (*v1alpha2.VirtualMachine, error) {
				return vm.DeepCopy(), nil
			},
			RestoreProvisionerFunc: func(context.Context, *corev1.Secret) (*corev1.Secret, error) {
				return nil, nil
			},
			RestoreVirtualMachineIPAddressFunc: func(context.Context, *corev1.Secret) (*v1alpha2.VirtualMachineIPAddress, error) {
				return nil, nil
			},
			RestoreVirtualMachineBlockDeviceAttachmentsFunc: func(context.Context, *corev1.Secret) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
				return nil, nil
			},
			RestoreVirtualMachineMACAddressesFunc: func(context.Context, *corev1.Secret) ([]*v1alpha2.VirtualMachineMACAddress, error) {
				return []*v1alpha2.VirtualMachineMACAddress{vmmac.DeepCopy()}, nil
			},
			RestoreMACAddressOrderFunc: func(context.Context, *corev1.Secret) ([]string, error) {
				return []string{"", "02:00:00:00:00:11"}, nil
			},
		}

		recorder := &eventrecord.EventRecorderLoggerMock{}
		recorder.EventFunc = func(client.Object, string, string, string) {}
		recorder.EventfFunc = func(client.Object, string, string, string, ...interface{}) {}
		recorder.AnnotatedEventfFunc = func(client.Object, map[string]string, string, string, string, ...interface{}) {}
		recorder.WithLoggingFunc = func(eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
			return recorder
		}

		handler := NewLifeCycleHandler(fakeClient, restorerMock, recorder)
		_, err = handler.Handle(testContext(), vmRestore)
		Expect(err).NotTo(HaveOccurred())

		restoredVM := &v1alpha2.VirtualMachine{}
		err = fakeClient.Get(testContext(), types.NamespacedName{Name: "vm", Namespace: "default"}, restoredVM)
		Expect(err).NotTo(HaveOccurred())

		Expect(restoredVM.Spec.Networks[0].VirtualMachineMACAddressName).To(BeEmpty())
		Expect(restoredVM.Spec.Networks[1].VirtualMachineMACAddressName).To(Equal("vm-mac-secondary"))
	})
})
