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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("StorageClassMigration", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f                      *framework.Framework
		ctx                    context.Context
		storageClass           *storagev1.StorageClass
		vi                     *v1alpha2.VirtualImage
		targetStorageClassName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("storage-class-migration")
		storageClass = framework.GetConfig().StorageClass.DefaultStorageClass
		if storageClass == nil {
			Skip("DefaultStorageClass is not set.")
		}
		targetStorageClass, err := getOrCreateTargetStorageClass(ctx, f, storageClass)
		Expect(err).NotTo(HaveOccurred())

		if targetStorageClass == "" {
			Skip("No available storage class for test")
		}
		targetStorageClassName = targetStorageClass

		f.Before()

		DeferCleanup(f.After)

		// The guest only needs to boot and report the agent, so the disks come
		// from the minimal custom image.
		newVI := object.NewGeneratedVIFromCVI("storage-class-migration-", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		newVI, err = f.VirtClient().VirtualImages(f.Namespace().Name).Create(ctx, newVI, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(newVI)
		vi = newVI
	})

	const (
		vdRootName       = "vd-alpine-root-disk"
		vdAdditionalName = "vd-alpine-additional-disk"
	)

	storageClassMigrationRootOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyRootBuild(f, vi, buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false, size: vdCustomImageSize})
	}

	storageClassMigrationRootAndLocalAdditionalBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false, size: vdCustomImageSize},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: true, size: vdCustomImageSize},
		)
	}

	storageClassMigrationRootAndAdditionalBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false, size: vdCustomImageSize},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: false, size: vdCustomImageSize},
		)
	}

	storageClassMigrationAdditionalOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: false, size: vdCustomImageSize},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: false, size: vdCustomImageSize},
		)
	}

	DescribeTable("should be successful", func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk), disksForMigration ...string) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		var vdsForMigration []*v1alpha2.VirtualDisk
		for _, vd := range vds {
			vd, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)

			if slices.Contains(disksForMigration, vd.Name) {
				vdsForMigration = append(vdsForMigration, vd)
			}
		}
		Expect(vdsForMigration).Should(HaveLen(len(disksForMigration)))

		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		// Armed before the trigger so no migration transition is missed.
		vdObservers := startVDObservers(ctx, f, vdsForMigration...)

		By("Patch VD with new storage class")
		denied, err := patchStorageClassNameTolerant(ctx, f, targetStorageClassName, vdsForMigration...)
		Expect(err).NotTo(HaveOccurred())

		By("Wait until VM migration succeeded")
		// The migration VMOP is created asynchronously by the controller with a
		// generated name, so it is discovered by a watch over the namespace's VMOPs.
		util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vm), framework.MaxTimeout)

		expectVDsMigrationSucceeded(ctx, f, vdObservers, vdsForMigration...)

		// A patch the webhook rejected mid-round (the disk was already migrating as
		// a same-storage-class companion) is re-applied once that round is over.
		if len(denied) > 0 {
			By("Re-patch VDs whose storage class change was rejected during the first round")
			err = patchStorageClassName(ctx, f, targetStorageClassName, denied...)
			Expect(err).NotTo(HaveOccurred())
		}

		for i, vdForMigration := range vdsForMigration {
			// The per-disk patches race with the controller: a disk whose patch came
			// second may have entered the first round as a same-storage-class
			// companion and settles on the source storage class; the controller then
			// runs an automatic follow-up round that carries it to the target one.
			// Wait until the disk actually reports the target storage class. The
			// fast-fail wrapper aborts the wait as soon as the follow-up round's
			// migration terminally fails instead of burning the whole timeout.
			err := vdObservers[i].WaitFor(vdobs.BeReadyWithStorageClass(targetStorageClassName), framework.MaxTimeout)
			if err != nil {
				skipIfKnownMigrationFailures(ctx, f)
			}
			Expect(err).NotTo(HaveOccurred(),
				"VirtualDisk %s/%s should settle on storage class %s", ns, vdForMigration.GetName(), targetStorageClassName)

			migratedVD, err := f.VirtClient().VirtualDisks(ns).Get(ctx, vdForMigration.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			pvc, err := f.KubeClient().CoreV1().PersistentVolumeClaims(ns).Get(ctx, migratedVD.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.StorageClassName).NotTo(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal(targetStorageClassName))
			Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound))
		}
	},
		Entry("when only root disk changed storage class", storageClassMigrationRootOnlyBuild, vdRootName),
		Entry("when root disk changed storage class and one local additional disk", storageClassMigrationRootAndLocalAdditionalBuild, vdRootName),
		Entry("when root disk changed storage class and one additional disk", storageClassMigrationRootAndAdditionalBuild, vdRootName, vdAdditionalName),
		// TODO: rnd and uncomment when problem will be solved
		// Entry("when only additional disk changed storage class", storageClassMigrationAdditionalOnlyBuild, vdAdditionalName),
	)

	// LabelTolerateFailedMigrations: the revert deliberately terminates the
	// volume migration with a failure, which the framework's failed-migration
	// fail-fast rules would otherwise abort the spec on.
	DescribeTable("should be reverted", Label(framework.LabelTolerateFailedMigrations), func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk), disksForMigration ...string) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		var vdsForMigration []*v1alpha2.VirtualDisk
		for _, vd := range vds {
			vd, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)

			if slices.Contains(disksForMigration, vd.Name) {
				vdsForMigration = append(vdsForMigration, vd)
			}
		}
		Expect(vdsForMigration).Should(HaveLen(len(disksForMigration)))

		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		// The revert below must land while the migration is still in flight, but
		// an idle guest with tiny disks migrates in seconds. stress-ng keeps
		// dirtying guest memory so the migration keeps re-copying and stays
		// revertable.
		ExecStressNGInVirtualMachine(f, vm)

		// Armed before the trigger so no migration transition is missed.
		vdObservers := startVDObservers(ctx, f, vdsForMigration...)

		By("Patch VD with new storage class")
		// A disk whose patch is rejected mid-round is already migrating in the same
		// round as a same-storage-class companion of the first patched disk; the
		// revert below rolls the whole volume set back either way.
		_, err = patchStorageClassNameTolerant(ctx, f, targetStorageClassName, vdsForMigration...)
		Expect(err).NotTo(HaveOccurred())

		By("Revert the migration as soon as it is in progress")
		// MaxTimeout: the revert specs run in parallel with the other migration
		// suites and queue behind kubevirt's parallelMigrationsPerCluster limit,
		// so the migration start alone can take several minutes.
		err = vmObs.WaitFor(vmobs.HaveMigrationInProgress(), framework.MaxTimeout)
		if err != nil {
			skipIfKnownMigrationFailures(ctx, f)
		}
		Expect(err).NotTo(HaveOccurred())

		// revert migration
		err = patchStorageClassName(ctx, f, storageClass.Name, vdsForMigration...)
		Expect(err).NotTo(HaveOccurred())

		expectVDsMigrationRevertedOrRolledBack(ctx, f, vdObservers, storageClass.Name, vdsForMigration...)
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
		err := f.CreateWithDeferredDeletion(ctx, objs...)
		Expect(err).NotTo(HaveOccurred())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		vdForMigration, err := f.VirtClient().VirtualDisks(ns).Get(ctx, vdRootName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		toStorageClasses := []string{targetStorageClassName, storageClass.Name}

		for _, sc := range toStorageClasses {
			By(fmt.Sprintf("Patch VD %s with new storage class %s", vdForMigration.Name, sc))

			// Armed before each round's trigger so its transitions are captured.
			vdObservers := startVDObservers(ctx, f, vdForMigration)

			err = patchStorageClassName(ctx, f, sc, vdForMigration)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Wait until the disk settles Ready on storage class %s", sc))
			// Wait on the disk's own end state rather than "a migration VMOP
			// completed": across two rounds the previous round's VMOP is still
			// present and Completed, so a watch for "any completed migration
			// VMOP" can match the stale one and return before this round's
			// migration finishes. BeReadyWithStorageClass is the deterministic
			// per-round signal. The fast-fail wrapper aborts the wait as soon as
			// this round's migration terminally fails instead of burning the
			// whole timeout.
			err = vdObservers[0].WaitFor(vdobs.BeReadyWithStorageClass(sc), framework.MaxTimeout)
			if err != nil {
				skipIfKnownMigrationFailures(ctx, f)
			}
			Expect(err).NotTo(HaveOccurred())

			migratedVD, err := f.VirtClient().VirtualDisks(ns).Get(ctx, vdForMigration.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			pvc, err := f.KubeClient().CoreV1().PersistentVolumeClaims(ns).Get(ctx, migratedVD.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Spec.StorageClassName).NotTo(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal(sc))
			Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound))
		}
	})
})

// storageClassKindByProvisioner maps a CSI provisioner to the Deckhouse
// storage CR kind (group storage.deckhouse.io/v1alpha1) that owns its
// StorageClasses: creating such a CR makes the storage module's controller
// create a StorageClass with the same name.
var storageClassKindByProvisioner = map[string]string{
	framework.SDSReplicatedVolume: "ReplicatedStorageClass",
	framework.SDSLocalVolume:      "LocalStorageClass",
	framework.NFS:                 "NFSStorageClass",
	// Both csi-ceph provisioners serve StorageClasses that sds-elastic owns
	// through an ElasticStorageClass named exactly like the StorageClass. A
	// Ceph class created via a bare csi-ceph CephStorageClass has no such CR,
	// and the NotFound fallback in getOrCreateTargetStorageClass covers it.
	framework.Ceph:   "ElasticStorageClass",
	framework.CephFS: "ElasticStorageClass",
}

// getOrCreateTargetStorageClass picks the migration target storage class.
//
// For provisioners managed through a Deckhouse storage CR the target is a
// fresh copy of the source class: an identical target keeps the migration
// about the volume move alone (same binding mode, same topology), while a
// randomly picked class can be structurally unmigratable — e.g. a node-local
// class with Immediate binding provisions the target volume on a node chosen
// without pod-placement awareness, and the target virt-launcher, forced to
// another node, can never attach it. For any other provisioner it falls back
// to picking a compatible existing class.
func getOrCreateTargetStorageClass(ctx context.Context, f *framework.Framework, storageClass *storagev1.StorageClass) (string, error) {
	if kind, ok := storageClassKindByProvisioner[storageClass.Provisioner]; ok {
		name, err := createTargetStorageClassCopy(ctx, f, kind, storageClass.Name)
		// A source class created by hand rather than through the CR has no CR
		// to copy; such a setup keeps the old behavior.
		if !k8serrors.IsNotFound(err) {
			return name, err
		}
	}
	return getTargetStorageClass(ctx, f, storageClass)
}

// createTargetStorageClassCopy clones the Deckhouse storage CR named after the
// source StorageClass under a generated name and waits until its controller
// creates the StorageClass. The clone is deleted after the spec — LIFO relative
// to the DeferCleanup(f.After) registered later in BeforeEach, so the namespace
// with all volumes of this class is gone first and the CR's protective
// finalizer does not block the deletion.
func createTargetStorageClassCopy(ctx context.Context, f *framework.Framework, kind, sourceName string) (string, error) {
	gvk := schema.GroupVersionKind{Group: "storage.deckhouse.io", Version: "v1alpha1", Kind: kind}

	source := &unstructured.Unstructured{}
	source.SetGroupVersionKind(gvk)
	if err := f.GenericClient().Get(ctx, crclient.ObjectKey{Name: sourceName}, source); err != nil {
		return "", fmt.Errorf("get source %s %s: %w", kind, sourceName, err)
	}

	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(gvk)
	target.SetGenerateName(framework.NamespaceBasePrefix + "-" + sourceName + "-")
	target.SetLabels(map[string]string{framework.E2ELabel: "true"})
	target.Object["spec"] = source.Object["spec"]
	if err := f.GenericClient().Create(ctx, target); err != nil {
		return "", fmt.Errorf("create %s copy of %s: %w", kind, sourceName, err)
	}

	DeferCleanup(func(ctx context.Context) error {
		if !framework.GetConfig().IsCleanupNeeded() {
			return nil
		}
		return f.Delete(ctx, target)
	})

	// The sds-elastic controller turns clones into StorageClasses strictly
	// serially at ~9s per clone (measured on an idle nightly nested ceph
	// cluster: the 8th of 8 concurrent clones appears after ~70s), and the
	// parallel suite creates all clones at once on an apiserver under peak
	// start-up load, so the wait must cover the whole queue with margin.
	_, err := observer.WaitForFirst(ctx, f.KubeClient().StorageV1().StorageClasses(), framework.LongTimeout,
		func(sc *storagev1.StorageClass) bool { return sc.Name == target.GetName() },
	)
	if err != nil {
		return "", fmt.Errorf("wait for StorageClass %s created from %s %s: %w", target.GetName(), kind, sourceName, err)
	}

	return target.GetName(), nil
}

func getTargetStorageClass(ctx context.Context, f *framework.Framework, storageClass *storagev1.StorageClass) (string, error) {
	// GetVolumeAndAccessModes needs no nil object.
	notEmptyVD := &v1alpha2.VirtualDisk{}
	modeGetter := volumemode.NewVolumeAndAccessModesGetter(f.GenericClient(), getStorageProfile(f))

	volumeMode, _, err := modeGetter.GetVolumeAndAccessModes(ctx, notEmptyVD, storageClass)
	if err != nil {
		return "", err
	}

	scList, err := f.KubeClient().StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	fallback := ""
	for _, sc := range scList.Items {
		if sc.Name == storageClass.Name {
			continue
		}
		// TODO: Add support for storage classes using the local volume provisioner.
		// Temporarily disabled because the storage layer itself has stability problems.
		// A local class is still a valid target when the source itself is served by
		// the local provisioner: the same-CSI preference below must see it.
		if sc.Provisioner == framework.SDSLocalVolume && sc.Provisioner != storageClass.Provisioner {
			GinkgoWriter.Printf("Skipping local storage class %s\n", sc.Name)
			continue
		}
		// The vd controller fast-fails a migration whose target carries this label
		// (BaseStorageClassService.IsStorageClassDeprecated).
		if sc.Labels["module"] == "local-path-provisioner" {
			GinkgoWriter.Printf("Skipping deprecated storage class %s\n", sc.Name)
			continue
		}

		nextVolumeMode, _, err := modeGetter.GetVolumeAndAccessModes(ctx, notEmptyVD, &sc)
		if err != nil {
			GinkgoWriter.Printf("Skipping storage class %s: cannot get volume mode: %s\n", sc.Name, err)
			continue
		}

		if volumeMode != nextVolumeMode {
			continue
		}

		// Prefer a target served by the same CSI driver as the source: the
		// migration then exercises only the volume move, not a second storage
		// backend whose own instability would fail the suite.
		if sc.Provisioner == storageClass.Provisioner {
			return sc.Name, nil
		}
		if fallback == "" {
			fallback = sc.Name
		}
	}
	return fallback, nil
}

// patchStorageClassNameTolerant patches each disk's storage class and returns
// the disks whose patch the vd webhook rejected because the controller had
// already started migrating them: the first patched disk's round picks up the
// VM's other local disks as same-storage-class companions within milliseconds,
// and mid-migration only a rollback is allowed.
//
// TODO: drop the tolerance if changing the storage class of several disks of
// one VM ever becomes atomic on the controller side; today the per-disk
// patches inherently race with the migration round started by the first one.
func patchStorageClassNameTolerant(ctx context.Context, f *framework.Framework, scName string, vds ...*v1alpha2.VirtualDisk) ([]*v1alpha2.VirtualDisk, error) {
	var denied []*v1alpha2.VirtualDisk
	for _, vd := range vds {
		err := patchStorageClassName(ctx, f, scName, vd)
		if err != nil {
			if strings.Contains(err.Error(), "storage class can be changed during migration only to rollback") {
				denied = append(denied, vd)
				continue
			}
			return nil, err
		}
	}
	return denied, nil
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

func getStorageProfile(f *framework.Framework) func(ctx context.Context, name string) (*storagev1alpha1.StorageProfile, error) {
	return func(ctx context.Context, name string) (*storagev1alpha1.StorageProfile, error) {
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

// expectVDsMigrationRevertedOrRolledBack waits for each VirtualDisk migration
// to fail (canceled by the storage-class revert). The e2e disks are tiny and
// can finish copying before the revert patch lands, leaving nothing to cancel:
// in that case the reverted spec migrates the disk back, so the helper accepts
// a succeeded first migration as long as the disk then settles Ready on the
// source storage class — the end state of a genuine revert.
func expectVDsMigrationRevertedOrRolledBack(ctx context.Context, f *framework.Framework, observers []vdobs.Observer, sourceStorageClassName string, vds ...*v1alpha2.VirtualDisk) {
	GinkgoHelper()

	By("Wait until VirtualDisks migrations failed")
	for i, obs := range observers {
		err := obs.WaitFor(vdobs.BeMigrationFailed(), framework.LongTimeout)
		if err != nil && strings.Contains(err.Error(), "migration succeeded, expected it to fail") {
			By(fmt.Sprintf("VirtualDisk %s migration finished before the revert, waiting for it to migrate back", vds[i].Name))
			err = obs.WaitFor(vdobs.BeReadyWithStorageClass(sourceStorageClassName), framework.LongTimeout)
		}
		if err != nil {
			skipIfKnownMigrationFailures(ctx, f)
		}
		Expect(err).NotTo(HaveOccurred(), "VirtualDisk %s/%s volume migration should fail and revert", vds[i].Namespace, vds[i].Name)
	}
}
