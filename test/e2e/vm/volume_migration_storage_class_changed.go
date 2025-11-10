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

package vm

import (
	"context"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// Ordered is required due to concurrent migration limitations in the cluster to prevent test interference.
// ContinueOnFailure ensures all independent tests run even if one fails.
var _ = Describe("StorageClassMigration", decoratorsForVolumeMigrations(), func() {
	var (
		f                      = framework.NewFramework("volume-migration-storage-class-changed")
		storageClass           *storagev1.StorageClass
		vi                     *v1alpha2.VirtualImage
		targetStorageClassName string
	)

	BeforeEach(func() {
		storageClass = framework.GetConfig().StorageClass.TemplateStorageClass
		if storageClass == nil {
			Skip("TemplateStorageClass is not set.")
		}
		targetStorageClass, err := getTargetStorageClass(f, storageClass)
		Expect(err).NotTo(HaveOccurred())

		if targetStorageClass == "" {
			Skip("No available storage class for test")
		}
		targetStorageClassName = targetStorageClass

		f.Before()

		DeferCleanup(f.After)

		newVI := object.NewGeneratedHTTPVIUbuntu("volume-migration-storage-class-changed-", f.Namespace().Name)
		newVI, err = f.VirtClient().VirtualImages(f.Namespace().Name).Create(context.Background(), newVI, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(newVI)
		vi = newVI
	})

	const (
		vdRootName       = "vd-ubuntu-root-disk"
		vdAdditionalName = "vd-ubuntu-additional-disk"
	)

	storageClassMigrationRootOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyRootBuild(f, vi, buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false})
	}

	storageClassMigrationRootAndLocalAdditionalBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: true},
		)
	}

	storageClassMigrationRootAndAdditionalBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: false},
		)
	}

	storageClassMigrationAdditionalOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: false},
		)
	}

	DescribeTable("should be successful", func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk), disksForMigration ...string) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		var vdsForMigration []*v1alpha2.VirtualDisk
		for _, vd := range vds {
			vd, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)

			if slices.Contains(disksForMigration, vd.Name) {
				vdsForMigration = append(vdsForMigration, vd)
			}
		}
		Expect(vdsForMigration).Should(HaveLen(len(disksForMigration)))

		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		By("Patch VD with new storage class")
		err = patchStorageClassName(context.Background(), f, targetStorageClassName, vdsForMigration...)
		Expect(err).NotTo(HaveOccurred())

		By("Wait until VM migration succeeded")
		util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vm), framework.MaxTimeout)

		untilVirtualDisksMigrationsSucceeded(f)

		for _, vdForMigration := range vdsForMigration {
			migratedVD, err := f.VirtClient().VirtualDisks(ns).Get(context.Background(), vdForMigration.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			pvc, err := f.KubeClient().CoreV1().PersistentVolumeClaims(ns).Get(context.Background(), migratedVD.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.StorageClassName).NotTo(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal(targetStorageClassName))
			Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound))
		}
	},
		Entry("when only root disk changed storage class", storageClassMigrationRootOnlyBuild, vdRootName),
		Entry("when root disk changed storage class and one local additional disk", storageClassMigrationRootAndLocalAdditionalBuild, vdRootName),
		Entry("when root disk changed storage class and one additional disk", storageClassMigrationRootAndAdditionalBuild, vdRootName, vdAdditionalName),
		Entry("when only additional disk changed storage class", storageClassMigrationAdditionalOnlyBuild, vdAdditionalName),
	)

	DescribeTable("should be reverted", func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk), disksForMigration ...string) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		var vdsForMigration []*v1alpha2.VirtualDisk
		for _, vd := range vds {
			vd, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)

			if slices.Contains(disksForMigration, vd.Name) {
				vdsForMigration = append(vdsForMigration, vd)
			}
		}
		Expect(vdsForMigration).Should(HaveLen(len(disksForMigration)))

		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		By("Patch VD with new storage class")
		err = patchStorageClassName(context.Background(), f, targetStorageClassName, vdsForMigration...)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			vm, err = f.VirtClient().VirtualMachines(ns).Get(context.Background(), vm.GetName(), metav1.GetOptions{})
			if err != nil {
				return err
			}

			state := vm.Status.MigrationState
			readyToCancel := state != nil && !state.StartTimestamp.IsZero() && state.EndTimestamp.IsZero()
			if !readyToCancel {
				return fmt.Errorf("migration is not in progress")
			}

			// revert migration
			err = patchStorageClassName(context.Background(), f, storageClass.Name, vdsForMigration...)
			Expect(err).NotTo(HaveOccurred())

			return nil
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())

		untilVirtualDisksMigrationsFailed(f)
	},
		Entry("when only root disk changed storage class", storageClassMigrationRootOnlyBuild, vdRootName),
		Entry("when root disk changed storage class and one local additional disk", storageClassMigrationRootAndLocalAdditionalBuild, vdRootName),
		Entry("when root disk changed storage class and one additional disk", storageClassMigrationRootAndAdditionalBuild, vdRootName, vdAdditionalName),
		Entry("when only additional disk changed storage class", storageClassMigrationAdditionalOnlyBuild, vdAdditionalName),
	)

	It("should be successful two migrations in a row", func() {
		ns := f.Namespace().Name

		vm, vds := storageClassMigrationRootAndAdditionalBuild()

		objs := []crclient.Object{vm}
		for _, vd := range vds {
			objs = append(objs, vd)
		}

		f.DeferDelete(objs...)
		err := f.CreateWithDeferredDeletion(context.Background(), objs...)
		Expect(err).NotTo(HaveOccurred())

		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		vdForMigration, err := f.VirtClient().VirtualDisks(ns).Get(context.Background(), vdRootName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		toStorageClasses := []string{targetStorageClassName, storageClass.Name}

		for _, sc := range toStorageClasses {
			By(fmt.Sprintf("Patch VD %s with new storage class %s", vdForMigration.Name, sc))

			err = patchStorageClassName(context.Background(), f, sc, vdForMigration)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				var lastVMOP *v1alpha2.VirtualMachineOperation
				vmops, err := f.VirtClient().VirtualMachineOperations(ns).List(context.Background(), metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())

				for _, vmop := range vmops.Items {
					if vmop.Spec.VirtualMachine == vm.Name {
						if lastVMOP == nil {
							lastVMOP = &vmop
							continue
						}
						if vmop.CreationTimestamp.After(lastVMOP.CreationTimestamp.Time) {
							lastVMOP = &vmop
							continue
						}
					}
				}

				if lastVMOP == nil {
					return fmt.Errorf("lastVMOP is not found")
				}

				if lastVMOP.Status.Phase == v1alpha2.VMOPPhaseCompleted {
					return nil
				}

				return fmt.Errorf("migration is not completed")
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())

			By("Wait until VM migration succeeded")
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vm), framework.MaxTimeout)

			untilVirtualDisksMigrationsSucceeded(f)

			migratedVD, err := f.VirtClient().VirtualDisks(ns).Get(context.Background(), vdForMigration.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			pvc, err := f.KubeClient().CoreV1().PersistentVolumeClaims(ns).Get(context.Background(), migratedVD.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.StorageClassName).NotTo(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal(sc))
			Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound))
		}
	})
})

func getTargetStorageClass(f *framework.Framework, storageClass *storagev1.StorageClass) (string, error) {
	// dirty hack to get volume mode. GetVolumeAndAccessModes needs no nil object.
	notEmptyVD := &v1alpha2.VirtualDisk{}
	modeGetter := volumemode.NewVolumeAndAccessModesGetter(f.Clients.GenericClient(), getStorageProfile(f))

	volumeMode, _, err := modeGetter.GetVolumeAndAccessModes(context.Background(), notEmptyVD, storageClass)
	if err != nil {
		return "", err
	}

	scList, err := f.KubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, sc := range scList.Items {
		if sc.Name == storageClass.Name {
			continue
		}
		// TODO: Add support for storage classes using the local volume provisioner.
		// Temporarily disabled because the storage layer itself has stability problems.
		if sc.Provisioner == "local.csi.storage.deckhouse.io" {
			GinkgoWriter.Printf("Skipping local storage class %s\n", sc.Name)
			continue
		}

		nextVolumeMode, _, err := modeGetter.GetVolumeAndAccessModes(context.Background(), notEmptyVD, &sc)
		if err != nil {
			GinkgoWriter.Printf("Skipping storage class %s: cannot get volume mode: %s\n", sc.Name, err)
			continue
		}

		if volumeMode == nextVolumeMode {
			return sc.Name, nil
		}
	}
	return "", nil
}

func patchStorageClassName(ctx context.Context, f *framework.Framework, scName string, vds ...*v1alpha2.VirtualDisk) error {
	patchBytes, err := patch.NewJSONPatch(patch.WithReplace("/spec/persistentVolumeClaim/storageClassName", scName)).Bytes()
	if err != nil {
		return fmt.Errorf("new json patch: %w", err)
	}

	for _, vd := range vds {
		_, err = f.VirtClient().VirtualDisks(vd.GetNamespace()).Patch(ctx, vd.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			return fmt.Errorf("patch vd %s %s: %w", vd.Name, string(patchBytes), err)
		}
	}

	return nil
}

func getStorageProfile(f *framework.Framework) func(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
	return func(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
		obj := &rewrite.StorageProfile{}
		err := f.RewriteClient().Get(ctx, name, obj)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return obj.StorageProfile, nil
	}
}
