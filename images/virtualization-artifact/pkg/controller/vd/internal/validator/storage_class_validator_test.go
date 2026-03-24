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

package validator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	basevc "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("StorageClassValidator", func() {
	var (
		fakeClient client.Client
		validator  *StorageClassValidator
		modeGetter *volumemode.VolumeAndAccessModesGetterMock
		scheme     *runtime.Scheme

		scName           = "test-sc"
		otherSCName      = "other-sc"
		deprecatedSCName = "deprecated-sc"
		vmName           = "test-vm"
		namespace        = "default"
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = v1alpha2.AddToScheme(scheme)
		_ = storagev1.AddToScheme(scheme)

		sc := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: scName,
			},
		}
		otherSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: otherSCName,
			},
		}
		deprecatedSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: deprecatedSCName,
				Labels: map[string]string{
					"module": "local-path-provisioner",
				},
			},
		}

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: namespace,
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
			},
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc, otherSC, deprecatedSC, vm).Build()
		baseSCService := basevc.NewBaseStorageClassService(fakeClient)
		vdSCService := service.NewVirtualDiskStorageClassService(baseSCService, config.VirtualDiskStorageClassSettings{})
		modeGetter = &volumemode.VolumeAndAccessModesGetterMock{
			GetVolumeAndAccessModesFunc: func(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
				return corev1.PersistentVolumeFilesystem, corev1.ReadWriteOnce, nil
			},
		}

		gate, setFromMap, err := featuregates.NewUnlocked()
		Expect(err).NotTo(HaveOccurred())
		err = setFromMap(map[string]bool{
			string(featuregates.VolumeMigration): true,
		})
		Expect(err).NotTo(HaveOccurred())

		validator = NewMigrationStorageClassValidator(fakeClient, vdSCService, modeGetter, gate)
	})

	DescribeTable("ValidateCreate", func(sc *string, wantErr bool) {
		vd := &v1alpha2.VirtualDisk{
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: sc,
				},
			},
		}
		_, err := validator.ValidateCreate(context.Background(), vd)
		if wantErr {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	},
		Entry("valid storage class", &scName, false),
		Entry("deprecated storage class", &deprecatedSCName, true),
		Entry("non-existent storage class", func() *string { s := "non-existent"; return &s }(), true),
		Entry("nil storage class", nil, false),
	)

	DescribeTable("ValidateUpdate", func(phase v1alpha2.DiskPhase, oldSC, newSC *string, wantErr bool) {
		oldVD := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Generation: 1, Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: oldSC,
				},
			},
		}
		newVD := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Generation: 2, Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: newSC,
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Phase: phase,
				StorageClassName: func() string {
					if oldSC != nil {
						return *oldSC
					}
					return ""
				}(),
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{
						Name:    vmName,
						Mounted: true,
					},
				},
			},
		}

		_, err := validator.ValidateUpdate(context.Background(), oldVD, newVD)
		if wantErr {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	},
		Entry("Pending: valid storage class", v1alpha2.DiskPending, nil, &scName, false),
		Entry("Pending: deprecated storage class", v1alpha2.DiskPending, nil, &deprecatedSCName, true),
		Entry("Pending: non-existent storage class", v1alpha2.DiskPending, nil, func() *string { s := "non-existent"; return &s }(), true),
		Entry("Pending: nil storage class", v1alpha2.DiskPending, &scName, nil, false),
		Entry("Migration: change SC (valid)", v1alpha2.DiskReady, &scName, &otherSCName, false),
		Entry("Migration: change SC (non-existent)", v1alpha2.DiskReady, &scName, func() *string { s := "non-existent"; return &s }(), true),
	)

	It("should fail migration if volume modes differ", func() {
		modeGetter.GetVolumeAndAccessModesFunc = func(ctx context.Context, obj client.Object, storageClass *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
			if storageClass.Name == scName {
				return corev1.PersistentVolumeFilesystem, corev1.ReadWriteOnce, nil
			}
			return corev1.PersistentVolumeBlock, corev1.ReadWriteOnce, nil
		}

		oldVD := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Generation: 1, Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: &scName,
				},
			},
		}
		newVD := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Generation: 2, Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: &otherSCName,
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:            v1alpha2.DiskReady,
				StorageClassName: scName,
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{
						Name:    vmName,
						Mounted: true,
					},
				},
			},
		}

		_, err := validator.ValidateUpdate(context.Background(), oldVD, newVD)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("different volume mode"))
	})

	It("should fail migration if VD is not mounted", func() {
		oldVD := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Generation: 1, Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: &scName,
				},
			},
		}
		newVD := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Generation: 2, Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: &otherSCName,
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:            v1alpha2.DiskReady,
				StorageClassName: scName,
				// AttachedToVirtualMachines is empty/nil by default, so Mounted will be false/missing
			},
		}
		_, err := validator.ValidateUpdate(context.Background(), oldVD, newVD)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not mounted"))
	})
})
