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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	e2eutil "github.com/deckhouse/virtualization/test/e2e/internal/util"
)

type buildOption struct {
	name         string
	storageClass *string
	rwo          bool
	// size overrides the disk size; empty means the default for the disk kind
	// (400Mi for a root disk from a VirtualImage, 100Mi for a blank disk).
	size string
}

func newRootVD(f *framework.Framework, root buildOption, vi *v1alpha2.VirtualImage) *v1alpha2.VirtualDisk {
	size := root.size
	if size == "" {
		size = "400Mi"
	}
	disk := object.NewVDFromVI(root.name, f.Namespace().Name, vi)
	vdbuilder.ApplyOptions(disk,
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
		vdbuilder.WithStorageClass(root.storageClass),
	)

	if root.rwo {
		vdbuilder.ApplyOptions(disk,
			vdbuilder.WithAnnotation(annotations.AnnVirtualDiskAccessMode, "ReadWriteOnce"),
		)
	}

	return disk
}

func newBlankVD(f *framework.Framework, additional buildOption) *v1alpha2.VirtualDisk {
	size := additional.size
	if size == "" {
		size = "100Mi"
	}
	blank := object.NewBlankVD(additional.name, f.Namespace().Name, additional.storageClass, ptr.To(resource.MustParse(size)))

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

// rootAndManyAdditionalBuild builds a VM with a root disk plus count additional
// ReadWriteOnce disks on the given storage class.
func rootAndManyAdditionalBuild(f *framework.Framework, vi *v1alpha2.VirtualImage, root buildOption, storageClass *string, count int) (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
	refs := []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.VirtualDiskKind, Name: root.name}}
	vds := []*v1alpha2.VirtualDisk{newRootVD(f, root, vi)}
	for i := range count {
		name := fmt.Sprintf("vd-alpine-additional-disk-%d", i)
		refs = append(refs, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.VirtualDiskKind, Name: name})
		vds = append(vds, newBlankVD(f, buildOption{name: name, storageClass: storageClass, rwo: true}))
	}
	vm := object.NewMinimalVM("volume-migration-many-disks-", f.Namespace().Name,
		vmbuilder.WithBlockDeviceRefs(refs...),
	)
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

// startVDObservers arms a VirtualDisk observer per disk. Call it before
// triggering a volume migration so the migration transitions (including a
// migration that finishes quickly) are captured for the expect* waits below.
func startVDObservers(ctx context.Context, f *framework.Framework, vds ...*v1alpha2.VirtualDisk) []vdobs.Observer {
	GinkgoHelper()
	observers := make([]vdobs.Observer, 0, len(vds))
	for _, vd := range vds {
		observers = append(observers, vdobs.StartObserver(ctx, f, vd))
	}
	return observers
}

// expectVDsMigrationSucceeded waits, via the pre-armed VirtualDisk observers,
// until every disk reports a finished, successful volume migration and has
// settled back on Ready with the target PVC in place.
func expectVDsMigrationSucceeded(ctx context.Context, f *framework.Framework, observers []vdobs.Observer, vds ...*v1alpha2.VirtualDisk) {
	GinkgoHelper()

	By("Wait until VirtualDisks migrations succeeded")
	for i, obs := range observers {
		err := obs.WaitFor(vdobs.BeMigrationSucceeded(), framework.MaxTimeout)
		if err != nil {
			skipIfKnownMigrationFailures(ctx, f)
		}
		Expect(err).NotTo(HaveOccurred(), "VirtualDisk %s/%s should finish its volume migration successfully", vds[i].Namespace, vds[i].Name)
	}
}

// expectVDsMigrationFailed waits, via the pre-armed VirtualDisk observers,
// until every disk reports a finished, failed volume migration and has been
// reverted to its source PVC.
func expectVDsMigrationFailed(ctx context.Context, f *framework.Framework, observers []vdobs.Observer, vds ...*v1alpha2.VirtualDisk) {
	GinkgoHelper()

	By("Wait until VirtualDisks migrations failed")
	for i, obs := range observers {
		err := obs.WaitFor(vdobs.BeMigrationFailed(), framework.LongTimeout)
		if err != nil {
			skipIfVolumeMigrationOutranTheCancel(err, vds[i])
			skipIfKnownMigrationFailures(ctx, f)
		}
		Expect(err).NotTo(HaveOccurred(), "VirtualDisk %s/%s volume migration should fail and revert", vds[i].Namespace, vds[i].Name)
	}
}

// skipIfVolumeMigrationOutranTheCancel skips the spec when the volume migration
// completed before the cancel could take effect, leaving nothing to revert.
//
// The revert specs cancel a migration that is already under way, so they need
// the copy to outlast the round trip between observing "migrating" and deleting
// the VirtualMachineOperation. How long that copy takes belongs to the storage
// class: one that provisions the target volume up front (Immediate binding)
// finishes a small disk in a blink, while WaitForFirstConsumer spends seconds
// creating the target PVC first and leaves a comfortable window. Losing that
// race says nothing about the revert path, so the spec steps aside instead of
// failing - and a cancel that stopped working altogether would show up as this
// spec never running at all.
func skipIfVolumeMigrationOutranTheCancel(err error, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()

	if err == nil || !strings.Contains(err.Error(), "migration succeeded, expected it to fail") {
		return
	}

	Skip(fmt.Sprintf("skip: volume migration of %s/%s completed before the cancel landed, the revert path was not exercised", vd.Namespace, vd.Name))
}

// skipIfKnownMigrationFailures skips the spec when any VM in the namespace
// shows a known infrastructure migration failure.
//
// TODO: remove temporary migration skip logic when both known issues are fixed:
// kubevirt "client socket is closed" and Volume(s)UpdateError.
func skipIfKnownMigrationFailures(ctx context.Context, f *framework.Framework) {
	GinkgoHelper()

	vms, err := f.VirtClient().VirtualMachines(f.Namespace().Name).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())
	for _, vm := range vms.Items {
		e2eutil.SkipIfKnownMigrationFailureWithContext(ctx, &vm)
	}
}

// EXCEPTION: this is an act-on-event loop, not a plain wait, and it operates
// on VMOPs created asynchronously by the controller with generated names the
// test cannot know in advance. The observer framework watches a single named
// object, so a polling wait is used deliberately here: each iteration re-lists
// the VMOPs and deletes those whose migration just started.
func untilVirtualMachinesWillBeStartMigratingAndCancelImmediately(f *framework.Framework) {
	GinkgoHelper()

	namespace := f.Namespace().Name

	someCompleted := false

	By("wait when migrations will be start migrating")
	eventually.Until(func() error {
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
			// TODO: remove temporary migration skip logic when both known issues are fixed:
			// kubevirt "client socket is closed" and Volume(s)UpdateError.
			e2eutil.SkipIfKnownMigrationFailure(&vm)
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
			case v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseSuperseded:
				someCompleted = true
				return nil
			}
		}
		return fmt.Errorf("retry because not all vmops canceled")
	},
		// MaxTimeout: the revert specs run in parallel with the other migration
		// suites and queue behind kubevirt's parallelMigrationsPerCluster limit,
		// so the migration start alone can take several minutes.
		framework.MaxTimeout)

	Expect(someCompleted).Should(BeFalse())
}
