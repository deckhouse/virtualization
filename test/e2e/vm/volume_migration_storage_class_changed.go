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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("StorageClassMigration", Ordered, framework.CommonE2ETestDecorators(), func() {
	var (
		f                = framework.NewFramework("volume-migration-storage-class-changed")
		storageClass     *storagev1.StorageClass
		vi               *v1alpha2.VirtualImage
		nextStorageClass string
	)

	BeforeEach(func() {
		// TODO: Remove Skip after fixing the issue.
		Skip("This test case is not working everytime. Should be fixed.")

		storageClass = framework.GetConfig().StorageClass.TemplateStorageClass
		if storageClass == nil {
			Skip("TemplateStorageClass is not set.")
		}

		scList, err := f.KubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())

		for _, sc := range scList.Items {
			if sc.Name == storageClass.Name {
				continue
			}
			if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
				nextStorageClass = sc.Name
				break
			}
		}

		if nextStorageClass == "" {
			Skip("No available storage class for test")
		}

		f.Before()

		DeferCleanup(f.After)

		newVI := object.NewGeneratedHTTPVIUbuntu("volume-migration-storage-class-changed-")
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
		err = PatchStorageClassName(context.Background(), f, nextStorageClass, vdsForMigration...)
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
			Expect(*pvc.Spec.StorageClassName).To(Equal(nextStorageClass))
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
		err = PatchStorageClassName(context.Background(), f, nextStorageClass, vdsForMigration...)
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
			err = PatchStorageClassName(context.Background(), f, storageClass.Name, vdsForMigration...)
			Expect(err).NotTo(HaveOccurred())

			return nil
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())

		untilVirtualDisksMigrationsFailed(f)
	},
		Entry("when only root disk changed storage class", storageClassMigrationRootOnlyBuild, vdRootName),
		Entry("when root disk changed storage class and one local additional disk", storageClassMigrationRootAndLocalAdditionalBuild, vdRootName),
		Entry("when root disk changed storage class and one additional disk", storageClassMigrationRootAndAdditionalBuild, vdRootName, vdAdditionalName), // TODO:fixme
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
		err := f.Create(context.Background(), objs...)
		Expect(err).NotTo(HaveOccurred())

		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		vdForMigration, err := f.VirtClient().VirtualDisks(ns).Get(context.Background(), vdRootName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		toStorageClasses := []string{nextStorageClass, storageClass.Name}

		for _, sc := range toStorageClasses {
			By(fmt.Sprintf("Patch VD %s with new storage class %s", vdForMigration.Name, sc))

			err = PatchStorageClassName(context.Background(), f, sc, vdForMigration)
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

func PatchStorageClassName(ctx context.Context, f *framework.Framework, scName string, vds ...*v1alpha2.VirtualDisk) error {
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
