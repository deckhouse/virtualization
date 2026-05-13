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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualDiskResizing", Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("virtual-disk-resizing")
		f.Before()
		DeferCleanup(f.After)
	})

	It("resizes virtual disks", func() {
		By("Environment preparation")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu, vd.WithSize(ptr.To(resource.MustParse("4Gi"))))
		vdBlank := object.NewBlankVD("vd-blank", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))
		vdAttach := object.NewBlankVD("vd-attach", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdRoot.Name,
				},
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdBlank.Name,
				},
			),
		)

		vmbda := object.NewVMBDAFromDisk("blank-disk-attachment", vm.Name, vdAttach)

		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdBlank, vdAttach, vm, vmbda)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		util.UntilSSHReady(f, vm, framework.LongTimeout)
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, vmbda)

		vdRootLsblkSize := util.GetBlockDeviceLsblkSize(ctx, f, vm, v1alpha2.VirtualDiskKind, vdRoot.Name)
		vdBlankLsblkSize := util.GetBlockDeviceLsblkSize(ctx, f, vm, v1alpha2.VirtualDiskKind, vdBlank.Name)
		vdAttachLsblkSize := util.GetBlockDeviceLsblkSize(ctx, f, vm, v1alpha2.VirtualDiskKind, vdAttach.Name)

		By("Resize the disks")
		ctxVDWatch, cancelVDWatch := context.WithCancel(ctx)
		defer cancelVDWatch()
		vdWatchErrCh := make(chan error, 1)
		vdWasResizingCh := make(chan bool, 1)

		go func() {
			wasResizing, err := ensureVDWasResizing(
				ctxVDWatch,
				f.VirtClient().VirtualDisks(f.Namespace().Name),
				[]*v1alpha2.VirtualDisk{vdRoot, vdBlank, vdAttach},
			)
			vdWasResizingCh <- wasResizing
			vdWatchErrCh <- err
		}()

		newVDRootSize, err := increaseDiskSize(ctx, f, vdRoot)
		Expect(err).NotTo(HaveOccurred())
		newVDBlankSize, err := increaseDiskSize(ctx, f, vdBlank)
		Expect(err).NotTo(HaveOccurred())
		newVDAttachSize, err := increaseDiskSize(ctx, f, vdAttach)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			wasResizing := false
			select {
			case v := <-vdWasResizingCh:
				wasResizing = v
			default:
			}
			return wasResizing
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(BeTrue())

		cancelVDWatch()
		Expect(<-vdWatchErrCh).ShouldNot(HaveOccurred())

		By("Verify that disks report the new size")
		util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.MiddleTimeout, vdRoot, vdBlank, vdAttach)
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.ShortTimeout, vm)
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, vmbda)

		err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vdRoot), vdRoot)
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vdBlank), vdBlank)
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vdAttach), vdAttach)
		Expect(err).NotTo(HaveOccurred())

		Expect(newVDRootSize.Cmp(resource.MustParse(vdRoot.Status.Capacity))).To(BeZero())
		Expect(newVDBlankSize.Cmp(resource.MustParse(vdBlank.Status.Capacity))).To(BeZero())
		Expect(newVDAttachSize.Cmp(resource.MustParse(vdAttach.Status.Capacity))).To(BeZero())

		newVDRootLsblkSize := util.GetBlockDeviceLsblkSize(ctx, f, vm, v1alpha2.VirtualDiskKind, vdRoot.Name)
		newVDBlankLsblkSize := util.GetBlockDeviceLsblkSize(ctx, f, vm, v1alpha2.VirtualDiskKind, vdBlank.Name)
		newVDAttachLsblkSize := util.GetBlockDeviceLsblkSize(ctx, f, vm, v1alpha2.VirtualDiskKind, vdAttach.Name)

		Expect(newVDRootLsblkSize).NotTo(Equal(vdRootLsblkSize))
		Expect(newVDBlankLsblkSize).NotTo(Equal(vdBlankLsblkSize))
		Expect(newVDAttachLsblkSize).NotTo(Equal(vdAttachLsblkSize))
	})
})

func increaseDiskSize(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) (resource.Quantity, error) {
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), vd)
	if err != nil {
		return resource.Quantity{}, err
	}

	if vd.Spec.PersistentVolumeClaim.Size == nil {
		return resource.Quantity{}, fmt.Errorf("virtual disk %s/%s must have PVC size in spec", vd.Namespace, vd.Name)
	}
	size := *vd.Spec.PersistentVolumeClaim.Size
	size.Add(resource.MustParse("1Gi"))
	vd.Spec.PersistentVolumeClaim.Size = ptr.To(size)

	err = f.GenericClient().Update(ctx, vd)
	if err != nil {
		return resource.Quantity{}, err
	}

	return size, nil
}

// ensureVDWasResizing watches VDs and returns true when each tracked VD
// reaches the Resizing phase at least once before context cancellation.
func ensureVDWasResizing(ctx context.Context, w util.Watcher, vds []*v1alpha2.VirtualDisk) (bool, error) {
	if len(vds) == 0 {
		return true, nil
	}

	tracked := make(map[string]struct{}, len(vds))
	seenResizing := make(map[string]struct{}, len(vds))
	for _, vd := range vds {
		if vd == nil {
			continue
		}
		tracked[vd.Name] = struct{}{}
		if vd.Status.Phase == v1alpha2.DiskResizing {
			seenResizing[vd.Name] = struct{}{}
		}
	}

	if len(tracked) == 0 || len(seenResizing) == len(tracked) {
		return true, nil
	}

	wi, err := w.Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	defer wi.Stop()

	for {
		select {
		case <-ctx.Done():
			return len(seenResizing) == len(tracked), nil
		case event, ok := <-wi.ResultChan():
			if !ok {
				if ctx.Err() != nil {
					return len(seenResizing) == len(tracked), nil
				}
				return false, fmt.Errorf("watch channel closed unexpectedly while VDs were still being monitored")
			}

			vd, ok := event.Object.(*v1alpha2.VirtualDisk)
			if !ok {
				continue
			}

			if _, isTracked := tracked[vd.Name]; !isTracked {
				continue
			}

			if vd.Status.Phase == v1alpha2.DiskResizing {
				seenResizing[vd.Name] = struct{}{}
				if len(seenResizing) == len(tracked) {
					return true, nil
				}
			}
		}
	}
}
