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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

const (
	httpStatusOk = "200"
	externalHost = "https://flant.ru"
)

type buildOption struct {
	name         string
	storageClass *string
	rwo          bool
}

func newRootVD(f *framework.Framework, root buildOption, vi *v1alpha2.VirtualImage) *v1alpha2.VirtualDisk {
	disk := object.NewVDFromVI(root.name, f.Namespace().Name, vi)
	vdbuilder.ApplyOptions(disk,
		vdbuilder.WithStorageClass(root.storageClass),
		vdbuilder.WithSize(ptr.To(resource.MustParse("2Gi"))),
	)

	if root.rwo {
		vdbuilder.ApplyOptions(disk,
			vdbuilder.WithAnnotation(annotations.AnnVirtualDiskAccessMode, "ReadWriteOnce"),
		)
	}

	return disk
}

func newBlankVD(f *framework.Framework, additional buildOption) *v1alpha2.VirtualDisk {
	blank := object.NewBlankVD(additional.name, f.Namespace().Name, additional.storageClass, ptr.To(resource.MustParse("100Mi")))

	if additional.rwo {
		vdbuilder.ApplyOptions(blank,
			vdbuilder.WithAnnotation(annotations.AnnVirtualDiskAccessMode, "ReadWriteOnce"),
		)
	}

	return blank
}

func onlyRootBuild(f *framework.Framework, vi *v1alpha2.VirtualImage, root buildOption) (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
	vm := object.NewMinimalVM("volume-migration-only-root-disk-", f.Namespace().Name,
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: root.name,
			},
		),
	)
	vds := []*v1alpha2.VirtualDisk{newRootVD(f, root, vi)}
	return vm, vds
}

func rootAndAdditionalBuild(f *framework.Framework, vi *v1alpha2.VirtualImage, root, additional buildOption) (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
	vm := object.NewMinimalVM("volume-migration-root-disk-and-additional-disk-", f.Namespace().Name,
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: root.name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: additional.name,
			},
		),
	)
	vds := []*v1alpha2.VirtualDisk{
		newRootVD(f, root, vi),
		newBlankVD(f, additional),
	}
	return vm, vds
}

func onlyAdditionalBuild(f *framework.Framework, vi *v1alpha2.VirtualImage, root, additional buildOption) (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
	vm := object.NewMinimalVM(
		"volume-migration-only-additional-disk-",
		f.Namespace().Name,
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: root.name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: additional.name,
			},
		),
	)
	vds := []*v1alpha2.VirtualDisk{
		newRootVD(f, root, vi),
		newBlankVD(f, additional),
	}
	return vm, vds
}

func untilVirtualDisksMigrationsSucceeded(f *framework.Framework) {
	GinkgoHelper()

	By("Wait until VirtualDisks migrations succeeded")
	Eventually(func(g Gomega) {
		vds, err := f.VirtClient().VirtualDisks(f.Namespace().Name).List(context.Background(), metav1.ListOptions{})
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(vds.Items).ShouldNot(BeEmpty())
		for _, vd := range vds.Items {
			g.Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
			g.Expect(vd.Status.Target.PersistentVolumeClaim).ShouldNot(BeEmpty())

			if vd.Status.MigrationState.StartTimestamp.IsZero() {
				// Skip the disks that are not migrated
				continue
			}

			g.Expect(vd.Status.MigrationState.EndTimestamp.IsZero()).Should(BeFalse(), "migration is not ended for vd %s", vd.Name)
			g.Expect(vd.Status.Target.PersistentVolumeClaim).To(Equal(vd.Status.MigrationState.TargetPVC))
			g.Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultSucceeded))
		}
	}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
}

func untilVirtualDisksMigrationsFailed(f *framework.Framework) {
	GinkgoHelper()

	By("Wait until VirtualDisks migrations failed")
	Eventually(func(g Gomega) {
		vds, err := f.VirtClient().VirtualDisks(f.Namespace().Name).List(context.Background(), metav1.ListOptions{})
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(vds.Items).ShouldNot(BeEmpty())
		for _, vd := range vds.Items {
			g.Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
			g.Expect(vd.Status.Target.PersistentVolumeClaim).ShouldNot(BeEmpty())

			if vd.Status.MigrationState.StartTimestamp.IsZero() {
				// Skip the disks that are not migrated
				continue
			}

			g.Expect(vd.Status.MigrationState.EndTimestamp.IsZero()).Should(BeFalse(), "migration is not ended for vd %s", vd.Name)
			g.Expect(vd.Status.MigrationState.SourcePVC).Should(Equal(vd.Status.Target.PersistentVolumeClaim))
			g.Expect(vd.Status.MigrationState.TargetPVC).ShouldNot(BeEmpty())
			g.Expect(vd.Status.MigrationState.Result).Should(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
		}
	}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())
}

func untilVirtualMachinesWillBeStartMigratingAndCancelImmediately(f *framework.Framework) {
	GinkgoHelper()

	namespace := f.Namespace().Name

	someCompleted := false

	By("wait when migrations will be start migrating")
	Eventually(func() error {
		vmops, err := f.VirtClient().VirtualMachineOperations(namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		if len(vmops.Items) == 0 {
			// All migrations were be canceled
			return nil
		}

		vms, err := f.VirtClient().VirtualMachines(namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		vmsByName := make(map[string]*v1alpha2.VirtualMachine, len(vms.Items))
		for _, vm := range vms.Items {
			vmsByName[vm.Name] = &vm
		}

		migrationReady := make(map[string]struct{})
		for _, vmop := range vmops.Items {
			if vm := vmsByName[vmop.Spec.VirtualMachine]; vm != nil {
				if vm.Status.MigrationState != nil && !vm.Status.MigrationState.StartTimestamp.IsZero() && vm.Status.MigrationState.EndTimestamp.IsZero() {
					migrationReady[vmop.Name] = struct{}{}
				}
			}
		}

		for _, vmop := range vmops.Items {
			switch vmop.Status.Phase {
			case v1alpha2.VMOPPhaseInProgress:
				_, readyToDelete := migrationReady[vmop.Name]

				if readyToDelete && vmop.GetDeletionTimestamp().IsZero() {
					err = f.VirtClient().VirtualMachineOperations(vmop.GetNamespace()).Delete(context.Background(), vmop.GetName(), metav1.DeleteOptions{})
					if err != nil {
						return err
					}
				}
			case v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseCompleted:
				someCompleted = true
				return nil
			}
		}
		return fmt.Errorf("retry because not all vmops canceled")
	}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())

	Expect(someCompleted).Should(BeFalse())
}
