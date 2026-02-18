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

package step

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

func TestSteps(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VMOP Snapshot Steps Suite")
}

func newNoOpRecorder() *eventrecord.EventRecorderLoggerMock {
	return &eventrecord.EventRecorderLoggerMock{
		EventFunc:           func(_ client.Object, _, _, _ string) {},
		EventfFunc:          func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		AnnotatedEventfFunc: func(_ client.Object, _ map[string]string, _, _, _ string, _ ...interface{}) {},
	}
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createVMSnapshot(namespace, name, secretName string, ready bool) *v1alpha2.VirtualMachineSnapshot {
	vms := &v1alpha2.VirtualMachineSnapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineSnapshotKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{
			VirtualMachineName: "test-vm",
		},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			Phase:                            v1alpha2.VirtualMachineSnapshotPhaseReady,
			VirtualMachineSnapshotSecretName: secretName,
		},
	}

	if ready {
		vms.Status.Conditions = []metav1.Condition{
			{
				Type:   string(vmscondition.VirtualMachineSnapshotReadyType),
				Status: metav1.ConditionTrue,
			},
		}
	}

	return vms
}

func createRestorerSecret(namespace, name string, vm *v1alpha2.VirtualMachine) *corev1.Secret {
	vmJSON, err := json.Marshal(vm)
	if err != nil {
		panic(err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"vm": vmJSON,
		},
		Type: "virtualmachine.virtualization.deckhouse.io/snapshot",
	}
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createRestoreVMOP(namespace, name, vmName, snapshotName string) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineOperationKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-vmop-uid"),
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeRestore,
			VirtualMachine: vmName,
			Restore: &v1alpha2.VirtualMachineOperationRestoreSpec{
				Mode:                       v1alpha2.SnapshotOperationModeStrict,
				VirtualMachineSnapshotName: snapshotName,
			},
		},
	}
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createCloneVMOP(namespace, name, vmName, snapshotName string) *v1alpha2.VirtualMachineOperation {
	vmop := &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineOperationKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-vmop-uid"),
			Annotations: map[string]string{
				annotations.AnnVMOPSnapshotName: snapshotName,
			},
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeClone,
			VirtualMachine: vmName,
			Clone: &v1alpha2.VirtualMachineOperationCloneSpec{
				Mode: v1alpha2.SnapshotOperationModeStrict,
				Customization: &v1alpha2.VirtualMachineOperationCloneCustomization{
					NameSuffix: "-clone",
				},
			},
		},
	}
	return vmop
}

func setMaintenanceCondition(vmop *v1alpha2.VirtualMachineOperation, status metav1.ConditionStatus) {
	vmop.Status.Conditions = append(vmop.Status.Conditions, metav1.Condition{
		Type:   string(vmopcondition.TypeMaintenanceMode),
		Status: status,
	})
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createVirtualDisk(namespace, name, ownerUID string, phase v1alpha2.DiskPhase) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualDisk",
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				annotations.AnnVMOPRestore: ownerUID,
			},
		},
		Status: v1alpha2.VirtualDiskStatus{
			Phase: phase,
		},
	}
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createVirtualMachine(namespace, name string, phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: v1alpha2.VirtualMachineStatus{
			Phase: phase,
		},
	}
}

//nolint:unparam // reason is always ReasonMaintenanceRestore in tests, but kept for flexibility
func setVMMaintenanceCondition(vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, reason vmcondition.MaintenanceReason) {
	vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
		Type:   string(vmcondition.TypeMaintenance),
		Status: status,
		Reason: string(reason),
	})
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createVMBDA(namespace, name, vmName string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return &v1alpha2.VirtualMachineBlockDeviceAttachment{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineBlockDeviceAttachmentKind,
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
			VirtualMachineName: vmName,
			BlockDeviceRef: v1alpha2.VMBDAObjectRef{
				Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
				Name: "test-disk",
			},
		},
	}
}

//nolint:unparam // namespace is always "default" in tests, but kept for flexibility
func createRestorerSecretWithVMBDAs(namespace, name string, vm *v1alpha2.VirtualMachine, vmbdas []*v1alpha2.VirtualMachineBlockDeviceAttachment) *corev1.Secret {
	secret := createRestorerSecret(namespace, name, vm)
	if len(vmbdas) > 0 {
		vmbdasJSON, err := json.Marshal(vmbdas)
		if err != nil {
			panic(err)
		}
		secret.Data["vmbdas"] = vmbdasJSON
	}
	return secret
}
