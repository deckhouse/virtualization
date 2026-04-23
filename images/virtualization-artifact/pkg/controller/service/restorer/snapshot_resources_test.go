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

package restorer

import (
	"context"
	"encoding/json"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SnapshotResources.Prepare", func() {
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
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, MAC: "02:00:00:00:00:11"},
				},
			},
		}

		vmmacs := []v1alpha2.VirtualMachineMACAddress{
			{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualMachineMACAddressKind,
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{Name: "vm-mac-secondary", Namespace: "default"},
				Status:     v1alpha2.VirtualMachineMACAddressStatus{Address: "02:00:00:00:00:11"},
			},
		}

		vmJSON, err := json.Marshal(vm)
		Expect(err).NotTo(HaveOccurred())

		vmmacsJSON, err := json.Marshal(vmmacs)
		Expect(err).NotTo(HaveOccurred())

		restorerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "restorer-secret", Namespace: "default"},
			Data: map[string][]byte{
				virtualMachineKey:             vmJSON,
				virtualMachineMACAddressesKey: vmmacsJSON,
			},
		}

		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"},
		}

		resources := NewSnapshotResources(
			fakeClient, v1alpha2.VMOPTypeRestore, v1alpha2.SnapshotOperationModeStrict,
			restorerSecret, vmSnapshot, "restore-uid",
		)
		Expect(resources.Prepare(context.Background())).To(Succeed())

		var restoredVM *v1alpha2.VirtualMachine
		for _, handler := range resources.GetObjectHandlers() {
			if obj, ok := handler.Object().(*v1alpha2.VirtualMachine); ok {
				restoredVM = obj
				break
			}
		}

		Expect(restoredVM).NotTo(BeNil())
		Expect(restoredVM.Spec.Networks[0].VirtualMachineMACAddressName).To(BeEmpty())
		Expect(restoredVM.Spec.Networks[1].VirtualMachineMACAddressName).To(Equal("vm-mac-secondary"))
	})

	DescribeTable("restores VirtualDisk requested size",
		func(originalSize, expectedSize *resource.Quantity) {
			vm := &v1alpha2.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualMachineKind,
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm",
					Namespace: "default",
				},
			}

			vmJSON, err := json.Marshal(vm)
			Expect(err).NotTo(HaveOccurred())

			restorerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "restorer-secret", Namespace: "default"},
				Data: map[string][]byte{
					virtualMachineKey: vmJSON,
				},
			}

			annotationsMap := map[string]string{}
			if originalSize != nil {
				annotationsMap[annotations.AnnVirtualDiskRequestedSize] = originalSize.String()
			}

			volumeSnapshot := &vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "volume-snapshot",
					Namespace:   "default",
					Annotations: annotationsMap,
				},
			}

			vdSnapshot := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vd-snapshot", Namespace: "default"},
				Spec: v1alpha2.VirtualDiskSnapshotSpec{
					VirtualDiskName: "vd",
				},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
					VolumeSnapshotName: volumeSnapshot.Name,
				},
			}

			scheme := runtime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			Expect(vsv1.AddToScheme(scheme)).To(Succeed())

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(volumeSnapshot, vdSnapshot).
				Build()

			vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
				Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"},
				Status: v1alpha2.VirtualMachineSnapshotStatus{
					VirtualDiskSnapshotNames: []string{vdSnapshot.Name},
				},
			}

			resources := NewSnapshotResources(
				fakeClient, v1alpha2.VMOPTypeRestore, v1alpha2.SnapshotOperationModeStrict,
				restorerSecret, vmSnapshot, "restore-uid",
			)
			Expect(resources.Prepare(context.Background())).To(Succeed())

			var restoredVD *v1alpha2.VirtualDisk
			for _, handler := range resources.GetObjectHandlers() {
				if obj, ok := handler.Object().(*v1alpha2.VirtualDisk); ok {
					restoredVD = obj
					break
				}
			}

			Expect(restoredVD).NotTo(BeNil())
			if expectedSize == nil {
				Expect(restoredVD.Spec.PersistentVolumeClaim.Size).To(BeNil())
			} else {
				Expect(restoredVD.Spec.PersistentVolumeClaim.Size).NotTo(BeNil())
				Expect(restoredVD.Spec.PersistentVolumeClaim.Size.String()).To(Equal(expectedSize.String()))
			}
		},
		Entry("keeps explicit size from snapshot metadata", ptrToQuantity("20Gi"), ptrToQuantity("20Gi")),
		Entry("leaves size empty when it was not set originally", nil, nil),
	)
})

func ptrToQuantity(value string) *resource.Quantity {
	q := resource.MustParse(value)
	return &q
}
