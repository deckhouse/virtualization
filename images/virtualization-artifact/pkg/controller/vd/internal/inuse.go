/*
Copyright 2024 Flant JSC

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
	"fmt"
	"slices"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var imagePhasesUsingDisk = []v1alpha2.ImagePhase{v1alpha2.ImageProvisioning, v1alpha2.ImagePending}

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	holders, err := h.updateAttachedVirtualMachines(ctx, vd)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Two instances with the volume mean two writers, left from before the owner election took in the
	// attachments and the instances. Nothing is taken from a running VM: the condition names all of them.
	inUseBy := mountedVirtualMachineNames(vd)
	if len(holders) > 1 {
		logger.FromContext(ctx).Warn("The virtual disk is plugged into several virtual machines", "vms", holders)
		inUseBy = holders
	}

	var (
		usedByVM         bool
		usedByImage      string
		usedByDataExport bool
	)

	usedByVM = h.checkUsageByVM(vd)
	if !usedByVM {
		usedByImage, err = h.checkImageUsage(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	if !usedByVM && usedByImage == "" {
		usedByDataExport, err = h.checkDataExportUsage(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// The messages name what holds the disk: this condition is the answer to "why does
	// the disk not detach" and, since a disk in use is protected from deletion, to
	// "why does the disk stay in Terminating".
	cb := conditions.NewConditionBuilder(vdcondition.InUseType).Generation(vd.Generation)
	switch {
	case usedByVM:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.AttachedToVirtualMachine).
			Message(service.InUseByVirtualMachinesMessage("VirtualDisk", inUseBy))
	case usedByImage != "":
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.UsedForImageCreation).
			Message(fmt.Sprintf("The VirtualDisk is in use for creating the %s; the creation must finish to release the disk.", usedByImage))
	case usedByDataExport:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.UsedForDataExport).
			Message("The VirtualDisk is in use by a data export request; the export must finish to release the disk.")
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.NotInUse).
			Message("")
	}

	conditions.SetCondition(cb, &vd.Status.Conditions)
	return reconcile.Result{}, nil
}

func (h InUseHandler) checkDataExportUsage(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	pvcName := vd.Status.Target.PersistentVolumeClaim
	if pvcName == "" {
		return false, nil
	}

	pvc, err := object.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, fmt.Errorf("fetch pvc: %w", err)
	}
	if pvc == nil {
		return false, nil
	}

	return annotations.IsDataExportRequested(pvc), nil
}

// checkImageUsage reports the image being created from the disk, e.g. `VirtualImage "golden"`,
// or an empty string when no image creation uses the disk. The kind and the name go into the
// InUse condition message so that the user knows which object to look at.
func (h InUseHandler) checkImageUsage(ctx context.Context, vd *v1alpha2.VirtualDisk) (string, error) {
	// If disk is not ready, it cannot be used for create image
	if vd.Status.Phase != v1alpha2.DiskReady {
		return "", nil
	}

	usedByImage, err := h.checkUsageByVI(ctx, vd)
	if err != nil {
		return "", err
	}
	if usedByImage == "" {
		usedByImage, err = h.checkUsageByCVI(ctx, vd)
		if err != nil {
			return "", err
		}
	}

	return usedByImage, nil
}

// updateAttachedVirtualMachines elects the disk owner and returns the VMs whose instances hold the volume.
func (h InUseHandler) updateAttachedVirtualMachines(ctx context.Context, vd *v1alpha2.VirtualDisk) ([]string, error) {
	var vms v1alpha2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting virtual machines: %w", err)
	}

	candidates, err := h.getOwnerCandidates(ctx, vd, vms)
	if err != nil {
		return nil, err
	}

	owner := electOwner(commonvd.GetCurrentlyMountedVMName(vd), candidates)

	// An image being created reads the disk: it is not given to a VM that does not hold the volume yet.
	if owner != "" && !slices.ContainsFunc(candidates, func(c ownerCandidate) bool { return c.name == owner && c.holds }) {
		usedByImage, err := h.checkImageUsage(ctx, vd)
		if err != nil {
			return nil, err
		}
		if usedByImage != "" {
			owner = ""
		}
	}

	var holders []string
	attachedVMs := make([]v1alpha2.AttachedVirtualMachine, 0, len(candidates))
	for _, c := range candidates {
		if c.holds {
			holders = append(holders, c.name)
		}
		// A VM that only waits for the disk through an attachment stays out of the list:
		// snapshots and WaitForFirstConsumer provisioning count the VMs listed here.
		if !c.listed && !c.holds && c.name != owner {
			continue
		}
		attachedVMs = append(attachedVMs, v1alpha2.AttachedVirtualMachine{Name: c.name, Mounted: c.name == owner})
	}

	vd.Status.AttachedToVirtualMachines = attachedVMs
	return holders, nil
}

// ownerCandidate is a VM that claims the disk or still has its volume.
type ownerCandidate struct {
	name string
	// listed: the VM status refers to the disk.
	listed bool
	// active: the VM is in a state that uses its disks.
	active bool
	// holds: the volume is on the instance of the VM or requested for it, whatever the spec says now.
	holds bool
}

// electOwner returns the VM that mounts the disk. A VM that holds the volume wins over one that only
// claims it, so the disk goes to another VM only once KubeVirt has really detached it; the current owner
// keeps the disk among equals, and the rest is decided by name to give the same answer on every pass.
func electOwner(current string, candidates []ownerCandidate) string {
	pick := func(eligible func(ownerCandidate) bool) string {
		first := ""
		for _, c := range candidates {
			if !eligible(c) {
				continue
			}
			if c.name == current {
				return current
			}
			if first == "" || c.name < first {
				first = c.name
			}
		}
		return first
	}

	if owner := pick(func(c ownerCandidate) bool { return c.holds }); owner != "" {
		return owner
	}

	return pick(func(c ownerCandidate) bool { return c.active })
}

func (h InUseHandler) getOwnerCandidates(ctx context.Context, vd *v1alpha2.VirtualDisk, vms v1alpha2.VirtualMachineList) ([]ownerCandidate, error) {
	claimedByAttachment, err := h.getVMsClaimingByAttachment(ctx, vd)
	if err != nil {
		return nil, err
	}

	holders, err := h.getVMsHoldingVolume(ctx, vd.GetNamespace(), kvbuilder.GenerateVDDiskName(vd.GetName()))
	if err != nil {
		return nil, err
	}

	var candidates []ownerCandidate
	for _, vm := range vms.Items {
		c := ownerCandidate{
			name:   vm.GetName(),
			listed: commonvm.HasBlockDeviceStatusRef(vm, v1alpha2.DiskDevice, vd.GetName()),
			holds:  holders[vm.GetName()],
		}

		if !c.listed && !claimedByAttachment[c.name] && !c.holds {
			continue
		}

		c.active, err = commonvm.UsesBlockDevices(ctx, h.client, vm)
		if err != nil {
			return nil, err
		}

		candidates = append(candidates, c)
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].name < candidates[j].name })

	return candidates, nil
}

// getVMsClaimingByAttachment returns the VMs whose attachments of the disk are still in force.
func (h InUseHandler) getVMsClaimingByAttachment(ctx context.Context, vd *v1alpha2.VirtualDisk) (map[string]bool, error) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := h.client.List(ctx, &vmbdas, &client.ListOptions{Namespace: vd.GetNamespace()})
	if err != nil {
		return nil, fmt.Errorf("error getting virtual machine block device attachments: %w", err)
	}

	claimed := make(map[string]bool)
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.BlockDeviceRef.Kind != v1alpha2.VMBDAObjectRefKindVirtualDisk || vmbda.Spec.BlockDeviceRef.Name != vd.GetName() {
			continue
		}
		// A conflicting attachment never plugs the disk, and a deleted one only unplugs it.
		if vmbda.DeletionTimestamp != nil || vmbda.Status.Phase == v1alpha2.BlockDeviceAttachmentPhaseFailed {
			continue
		}
		claimed[vmbda.Spec.VirtualMachineName] = true
	}

	return claimed, nil
}

// getVMsHoldingVolume returns the VMs whose running instance has the volume or an attach request for it
// waits on the internal VM. The VM status is built from the internal VM template, which loses the volume
// before the guest releases it, and a request goes through even if the attachment behind it is deleted.
// The lookups go through indexes: a namespace of a nested cluster holds all its nodes.
func (h InUseHandler) getVMsHoldingVolume(ctx context.Context, namespace, volumeName string) (map[string]bool, error) {
	holders := make(map[string]bool)

	var kvvmis virtv1.VirtualMachineInstanceList
	err := h.client.List(ctx, &kvvmis, client.InNamespace(namespace), client.MatchingFields{indexer.IndexFieldKVVMIByVolume: volumeName})
	if err != nil {
		return nil, fmt.Errorf("list the internal virtual machine instances with the volume: %w", err)
	}
	for _, kvvmi := range kvvmis.Items {
		holders[kvvmi.Name] = true
	}

	var kvvms virtv1.VirtualMachineList
	err = h.client.List(ctx, &kvvms, client.InNamespace(namespace), client.MatchingFields{indexer.IndexFieldKVVMByAddVolumeRequest: volumeName})
	if err != nil {
		return nil, fmt.Errorf("list the internal virtual machines with an attach request for the volume: %w", err)
	}
	for _, kvvm := range kvvms.Items {
		if holders[kvvm.Name] {
			continue
		}
		// A request holds the disk only for a running instance: a stopped VM gets the volume on its next start.
		kvvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: kvvm.Name, Namespace: namespace}, h.client, &virtv1.VirtualMachineInstance{})
		if err != nil {
			return nil, fmt.Errorf("fetch the internal virtual machine instance: %w", err)
		}
		if kvvmi != nil {
			holders[kvvm.Name] = true
		}
	}

	return holders, nil
}

func (h InUseHandler) checkUsageByVM(vd *v1alpha2.VirtualDisk) bool {
	for _, attachedVM := range vd.Status.AttachedToVirtualMachines {
		if attachedVM.Mounted {
			return true
		}
	}

	return false
}

func (h InUseHandler) checkUsageByVI(ctx context.Context, vd *v1alpha2.VirtualDisk) (string, error) {
	var vis v1alpha2.VirtualImageList
	err := h.client.List(ctx, &vis, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return "", fmt.Errorf("error getting virtual images: %w", err)
	}

	names := make([]string, 0, len(vis.Items))
	for _, vi := range vis.Items {
		if slices.Contains(imagePhasesUsingDisk, vi.Status.Phase) &&
			vi.Spec.DataSource.Type == v1alpha2.DataSourceTypeObjectRef &&
			vi.Spec.DataSource.ObjectRef != nil &&
			vi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskKind &&
			vi.Spec.DataSource.ObjectRef.Name == vd.Name {
			names = append(names, vi.GetName())
		}
	}

	// Several images may be created from the same disk: report the first one by name so
	// that the message does not depend on the list order returned by the client cache.
	if len(names) == 0 {
		return "", nil
	}

	sort.Strings(names)

	return fmt.Sprintf("%s %q", v1alpha2.VirtualImageKind, names[0]), nil
}

func (h InUseHandler) checkUsageByCVI(ctx context.Context, vd *v1alpha2.VirtualDisk) (string, error) {
	var cvis v1alpha2.ClusterVirtualImageList
	err := h.client.List(ctx, &cvis, &client.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting cluster virtual images: %w", err)
	}

	names := make([]string, 0, len(cvis.Items))
	for _, cvi := range cvis.Items {
		if slices.Contains(imagePhasesUsingDisk, cvi.Status.Phase) &&
			cvi.Spec.DataSource.Type == v1alpha2.DataSourceTypeObjectRef &&
			cvi.Spec.DataSource.ObjectRef != nil &&
			cvi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskKind &&
			cvi.Spec.DataSource.ObjectRef.Name == vd.Name &&
			cvi.Spec.DataSource.ObjectRef.Namespace == vd.Namespace {
			names = append(names, cvi.GetName())
		}
	}

	if len(names) == 0 {
		return "", nil
	}

	sort.Strings(names)

	return fmt.Sprintf("%s %q", v1alpha2.ClusterVirtualImageKind, names[0]), nil
}

// mountedVirtualMachineNames returns the sorted names of the VirtualMachines that
// currently mount the disk.
func mountedVirtualMachineNames(vd *v1alpha2.VirtualDisk) []string {
	names := make([]string, 0, len(vd.Status.AttachedToVirtualMachines))
	for _, vm := range vd.Status.AttachedToVirtualMachines {
		if vm.Mounted {
			names = append(names, vm.Name)
		}
	}

	sort.Strings(names)

	return names
}
