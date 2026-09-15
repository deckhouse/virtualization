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

package restorer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	restorer "github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/restorers"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ResourceStatusPhase string

const (
	ResourceStatusInProgress ResourceStatusPhase = "InProgress"
	ResourceStatusCompleted  ResourceStatusPhase = "Completed"
	ResourceStatusFailed     ResourceStatusPhase = "Failed"
)

type SnapshotResourceStatus struct {
	APIVersion string
	Kind       string
	Name       string
	Status     ResourceStatusPhase
	Message    string
}

type SnapshotResources struct {
	uuid           string
	client         client.Client
	manifestReader ManifestReader
	vmSnapshot     *v1alpha2.VirtualMachineSnapshot
	objectHandlers []ObjectHandler
	statuses       []v1alpha2.SnapshotResourceStatus
	mode           v1alpha2.SnapshotOperationMode
	kind           v1alpha2.VMOPType
}

func NewSnapshotResources(client client.Client, kind v1alpha2.VMOPType, mode v1alpha2.SnapshotOperationMode, manifestReader ManifestReader, vmSnapshot *v1alpha2.VirtualMachineSnapshot, uuid string) SnapshotResources {
	return SnapshotResources{
		mode:           mode,
		kind:           kind,
		uuid:           uuid,
		client:         client,
		manifestReader: manifestReader,
		vmSnapshot:     vmSnapshot,
	}
}

func (r *SnapshotResources) Prepare(ctx context.Context) error {
	provisioner, err := r.manifestReader.RestoreProvisioner(ctx)
	if err != nil {
		return err
	}

	vm, err := r.manifestReader.RestoreVirtualMachine(ctx)
	if err != nil {
		return err
	}

	vmip, err := r.manifestReader.RestoreVirtualMachineIPAddress(ctx)
	if err != nil {
		return err
	}

	if vmip != nil && r.kind == v1alpha2.VMOPTypeRestore {
		vm.Spec.VirtualMachineIPAddress = vmip.Name
	} else {
		vm.Spec.VirtualMachineIPAddress = ""
	}

	vmmacs, err := r.manifestReader.RestoreVirtualMachineMACAddresses(ctx)
	if err != nil {
		return err
	}

	macAddressOrder, err := r.manifestReader.RestoreMACAddressOrder(ctx)
	if err != nil {
		return err
	}

	vds, err := getVirtualDisks(ctx, r.client, r.vmSnapshot, vm.Name, r.kind)
	if err != nil {
		return err
	}

	vmbdas, err := r.manifestReader.RestoreVirtualMachineBlockDeviceAttachments(ctx)
	if err != nil {
		return err
	}

	r.confineToSnapshotNamespace(vm, vmip, provisioner)
	for _, vmmac := range vmmacs {
		r.confineToSnapshotNamespace(vmmac)
	}
	for _, vd := range vds {
		r.confineToSnapshotNamespace(vd)
	}
	for _, vmbda := range vmbdas {
		r.confineToSnapshotNamespace(vmbda)
	}

	if len(vmmacs) > 0 && r.kind == v1alpha2.VMOPTypeRestore {
		macAddressNamesByAddress := make(map[string]string)
		for _, vmmac := range vmmacs {
			r.objectHandlers = append(r.objectHandlers, restorer.NewVirtualMachineMACAddressHandler(r.client, vmmac, r.uuid))
			macAddressNamesByAddress[vmmac.Status.Address] = vmmac.Name
		}

		if len(macAddressOrder) < len(vm.Spec.Networks) {
			return fmt.Errorf(
				"captured virtual machine %q declares %d networks in spec but only %d in status: cannot restore the MAC address order",
				vm.Name, len(vm.Spec.Networks), len(macAddressOrder),
			)
		}

		for i := range vm.Spec.Networks {
			ns := &vm.Spec.Networks[i]
			if ns.Type == v1alpha2.NetworksTypeMain {
				continue
			}

			ns.VirtualMachineMACAddressName = macAddressNamesByAddress[macAddressOrder[i]]
		}
	} else {
		for i := range vm.Spec.Networks {
			vm.Spec.Networks[i].VirtualMachineMACAddressName = ""
		}
	}

	if vmip != nil {
		r.objectHandlers = append(r.objectHandlers, restorer.NewVirtualMachineIPAddressHandler(r.client, vmip, r.uuid))
	}

	for _, vd := range vds {
		r.objectHandlers = append(r.objectHandlers, restorer.NewVirtualDiskHandler(r.client, *vd, r.uuid))
	}

	for _, vmbda := range vmbdas {
		r.objectHandlers = append(r.objectHandlers, restorer.NewVMBlockDeviceAttachmentHandler(r.client, *vmbda, r.uuid))
	}

	if provisioner != nil {
		r.objectHandlers = append(r.objectHandlers, restorer.NewProvisionerHandler(r.client, *provisioner, r.uuid))
	}

	r.objectHandlers = append(r.objectHandlers, restorer.NewVirtualMachineHandler(r.client, *vm, r.uuid, r.mode))

	return nil
}

// confineToSnapshotNamespace pins objects a restore is about to create to the namespace of the snapshot
// being restored, ignoring any that were not captured at all.
//
// These objects are decoded from captured manifests, and a captured manifest carries the
// metadata.namespace it had when it was taken. For a snapshot captured in place that is the same
// namespace it is restored into, so nothing ever noticed; an imported snapshot breaks the coincidence,
// because it lives wherever the archive was loaded while its manifests still name the namespace the
// original was captured from. Restoring one then recreated the VirtualMachine back in that original
// namespace — one the requester may have no access to at all, and one this controller can write to
// regardless, since it holds cluster-wide credentials.
//
// It is the same rule the restore subresource enforces on its own callers: a snapshot is answerable
// only for its own namespace. assertConfinedToSnapshotNamespace is what keeps a resource added later
// from quietly escaping it.
func (r *SnapshotResources) confineToSnapshotNamespace(objs ...client.Object) {
	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}
		obj.SetNamespace(r.vmSnapshot.Namespace)
	}
}

// assertConfinedToSnapshotNamespace reports an object a restore would create outside the snapshot's
// namespace. Nothing should reach it — the objects are pinned as they are read — so it exists for the
// resource somebody adds later and forgets to pin: it turns a cross-namespace write into a refusal with
// a name attached, rather than a silent one.
func (r *SnapshotResources) assertConfinedToSnapshotNamespace(obj client.Object) error {
	if obj.GetNamespace() == r.vmSnapshot.Namespace {
		return nil
	}
	return fmt.Errorf(
		"refusing to restore %s %q into namespace %q: a snapshot restores only into its own namespace, %q",
		obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), obj.GetNamespace(), r.vmSnapshot.Namespace,
	)
}

func (r *SnapshotResources) Override(rules []v1alpha2.NameReplacement) {
	for _, ov := range r.objectHandlers {
		ov.Override(rules)
	}
}

func (r *SnapshotResources) Customize(prefix, suffix string) {
	for _, ov := range r.objectHandlers {
		ov.Customize(prefix, suffix)
	}
}

func (r *SnapshotResources) Validate(ctx context.Context) ([]v1alpha2.SnapshotResourceStatus, error) {
	var hasErrors bool

	r.statuses = make([]v1alpha2.SnapshotResourceStatus, 0, len(r.objectHandlers))

	for _, ov := range r.objectHandlers {
		obj := ov.Object()

		status := v1alpha2.SnapshotResourceStatus{
			APIVersion: obj.GetObjectKind().GroupVersionKind().Version,
			Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Status:     v1alpha2.SnapshotResourceStatusCompleted,
			Message:    obj.GetName() + " is valid for restore",
		}

		if err := r.assertConfinedToSnapshotNamespace(obj); err != nil {
			hasErrors = true
			status.Status = v1alpha2.SnapshotResourceStatusFailed
			status.Message = err.Error()
			r.statuses = append(r.statuses, status)
			continue
		}

		switch r.kind {
		case v1alpha2.VMOPTypeRestore:
			err := ov.ValidateRestore(ctx)
			switch {
			case err == nil:
			case shouldIgnoreError(r.mode, err):
			default:
				hasErrors = true
				status.Status = v1alpha2.SnapshotResourceStatusFailed
				status.Message = err.Error()
			}
		case v1alpha2.VMOPTypeClone:
			err := ov.ValidateClone(ctx)
			if err != nil {
				hasErrors = true
				status.Status = v1alpha2.SnapshotResourceStatusFailed
				status.Message = err.Error()
			}
		}
		r.statuses = append(r.statuses, status)
	}

	if hasErrors {
		return r.statuses, errors.New("fail to validate the resources: check the status")
	}

	return r.statuses, nil
}

func (r *SnapshotResources) Process(ctx context.Context) ([]v1alpha2.SnapshotResourceStatus, error) {
	var hasErrors bool

	r.statuses = make([]v1alpha2.SnapshotResourceStatus, 0, len(r.objectHandlers))

	if r.mode == v1alpha2.SnapshotOperationModeDryRun {
		return r.statuses, errors.New("cannot Process with DryRun operation")
	}

	for _, ov := range r.objectHandlers {
		obj := ov.Object()

		status := v1alpha2.SnapshotResourceStatus{
			APIVersion: obj.GetObjectKind().GroupVersionKind().Version,
			Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Status:     v1alpha2.SnapshotResourceStatusCompleted,
			Message:    "Successfully processed",
		}

		switch r.kind {
		case v1alpha2.VMOPTypeRestore:
			err := ov.ProcessRestore(ctx)
			switch {
			case err == nil:
			case shouldIgnoreError(r.mode, err):
			case isRetryError(err):
				status.Status = v1alpha2.SnapshotResourceStatusInProgress
				status.Message = err.Error()
			default:
				hasErrors = true
				status.Status = v1alpha2.SnapshotResourceStatusFailed
				status.Message = err.Error()
			}
		case v1alpha2.VMOPTypeClone:
			err := ov.ProcessClone(ctx)
			switch {
			case err == nil:
			case isRetryError(err):
				status.Status = v1alpha2.SnapshotResourceStatusInProgress
				status.Message = err.Error()
			default:
				hasErrors = true
				status.Status = v1alpha2.SnapshotResourceStatusFailed
				status.Message = err.Error()
			}
		}
		r.statuses = append(r.statuses, status)
	}

	if hasErrors {
		return r.statuses, errors.New("fail to process the resources: check the status")
	}

	vmKey, vdKeys := r.getRestoredVMAndVDKeys()
	vm := &v1alpha2.VirtualMachine{}
	if err := r.client.Get(ctx, vmKey, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return r.statuses, common.ErrQueueing
		}
		return r.statuses, fmt.Errorf("failed to get virtual machine %s: %w", vmKey, err)
	}
	for _, vdKey := range vdKeys {
		if err := r.setOwnerRefOnVirtualDisk(ctx, vm, vdKey); err != nil {
			return r.statuses, err
		}
	}

	return r.statuses, nil
}

var BestEffortIgnoredErrors = []error{
	common.ErrVirtualImageNotFound,
	common.ErrClusterVirtualImageNotFound,
	common.ErrSecretHasDifferentData,
}

var RetryErrors = []error{
	common.ErrRestoring,
	common.ErrUpdating,
	common.ErrWaitingForDeletion,
}

func shouldIgnoreError(mode v1alpha2.SnapshotOperationMode, err error) bool {
	if mode == v1alpha2.SnapshotOperationModeBestEffort {
		for _, e := range BestEffortIgnoredErrors {
			if errors.Is(err, e) {
				return true
			}
		}
	}

	return false
}

func isRetryError(err error) bool {
	if apierrors.IsConflict(err) {
		return true
	}

	for _, e := range RetryErrors {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// virtualDiskSnapshotNames returns the names of vmSnapshot's VirtualDiskSnapshot children. The built-in
// mechanism tracks these in Status.VirtualDiskSnapshotNames. The unified-snapshotter SDK controllers
// track children generically in Status.ChildrenSnapshotRefs (written by the SDK's EnsureChildren,
// independent of our own patchStatus's owned-field list).
func virtualDiskSnapshotNames(vmSnapshot *v1alpha2.VirtualMachineSnapshot) []string {
	if !IsUnifiedCapture(vmSnapshot) {
		return vmSnapshot.Status.VirtualDiskSnapshotNames
	}
	names := make([]string, 0, len(vmSnapshot.Status.ChildrenSnapshotRefs))
	for _, ref := range vmSnapshot.Status.ChildrenSnapshotRefs {
		if ref.APIVersion == v1alpha2.SchemeGroupVersion.String() && ref.Kind == v1alpha2.VirtualDiskSnapshotKind {
			names = append(names, ref.Name)
		}
	}
	return names
}

// getVirtualDisks builds the VirtualDisks a restore has to create, one per VirtualDiskSnapshot child.
// vmName is the name of the VirtualMachine being restored, read from its captured manifest: the snapshot
// does not necessarily know it, because an imported one records no source at all.
func getVirtualDisks(ctx context.Context, client client.Client, vmSnapshot *v1alpha2.VirtualMachineSnapshot, vmName string, kind v1alpha2.VMOPType) ([]*v1alpha2.VirtualDisk, error) {
	vdSnapshotNames := virtualDiskSnapshotNames(vmSnapshot)
	vds := make([]*v1alpha2.VirtualDisk, 0, len(vdSnapshotNames))

	for _, vdSnapshotName := range vdSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch the virtual disk snapshot %q: %w", vdSnapshotKey.Name, err)
		}

		if vdSnapshot == nil {
			return nil, fmt.Errorf("failed to get the virtual disk snapshot %q: %w", vdSnapshotName, common.ErrVirtualDiskSnapshotNotFound)
		}

		// Set AttachedToVirtualMachines only for restore operation.
		// For clone operation, leave it empty so WaitForFirstConsumer logic works correctly.
		var attachedVMs []v1alpha2.AttachedVirtualMachine
		if kind == v1alpha2.VMOPTypeRestore {
			attachedVMs = []v1alpha2.AttachedVirtualMachine{
				{Name: vmName, Mounted: true},
			}
		}

		vd := v1alpha2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualDiskKind,
				APIVersion: v1alpha2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdSnapshot.SourceVirtualDiskName(),
				Namespace: vdSnapshot.Namespace,
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				AttachedToVirtualMachines: attachedVMs,
			},
		}

		// Leaves vd.Name alone when the spec already resolved one, and fills it from the captured
		// manifest otherwise — see AddOriginalMetadata.
		err = AddOriginalMetadata(ctx, &vd, vdSnapshot, client)
		if err != nil {
			return nil, fmt.Errorf("failed to add original metadata: %w", err)
		}

		if vd.Name == "" {
			return nil, fmt.Errorf(
				"cannot tell what the virtual disk captured by %q was called: the snapshot names no source and its captured manifest holds no VirtualDisk",
				vdSnapshot.Name,
			)
		}

		vds = append(vds, &vd)
	}

	return vds, nil
}

func (r *SnapshotResources) GetObjectHandlers() []ObjectHandler {
	return r.objectHandlers
}

func (r *SnapshotResources) getRestoredVMAndVDKeys() (types.NamespacedName, []types.NamespacedName) {
	var vmKey types.NamespacedName
	vdKeys := make([]types.NamespacedName, 0)
	for _, ov := range r.objectHandlers {
		obj := ov.Object()
		kind := obj.GetObjectKind().GroupVersionKind().Kind
		key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
		switch kind {
		case v1alpha2.VirtualMachineKind:
			vmKey = key
		case v1alpha2.VirtualDiskKind:
			vdKeys = append(vdKeys, key)
		}
	}
	return vmKey, vdKeys
}

func (r *SnapshotResources) setOwnerRefOnVirtualDisk(ctx context.Context, vm *v1alpha2.VirtualMachine, vdKey types.NamespacedName) error {
	vd := &v1alpha2.VirtualDisk{}
	if err := r.client.Get(ctx, vdKey, vd); err != nil {
		if apierrors.IsNotFound(err) {
			return common.ErrQueueing
		}
		return fmt.Errorf("failed to get virtual disk %s: %w", vdKey, err)
	}

	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return nil
	}

	if vd.Annotations[annotations.AnnVMOPRestore] != r.uuid || len(vd.OwnerReferences) > 0 {
		return nil
	}

	vdSnapshotName := vd.Spec.DataSource.ObjectRef.Name
	vdSnapshotKey := types.NamespacedName{Namespace: vd.Namespace, Name: vdSnapshotName}
	vdSnapshot := &v1alpha2.VirtualDiskSnapshot{}
	if err := r.client.Get(ctx, vdSnapshotKey, vdSnapshot); err != nil {
		return fmt.Errorf("failed to get virtual disk snapshot %s: %w", vdSnapshotKey, err)
	}

	hadOwnerReference, err := r.virtualDiskHadOwnerReference(ctx, vd.Namespace, vdSnapshot)
	if err != nil {
		return err
	}
	if !hadOwnerReference {
		return nil
	}

	vd.OwnerReferences = append(vd.OwnerReferences, metav1.OwnerReference{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineKind,
		Name:       vm.Name,
		UID:        vm.UID,
	})
	if err := r.client.Update(ctx, vd); err != nil {
		if apierrors.IsConflict(err) {
			return common.ErrQueueing
		}
		return err
	}

	return nil
}

func (r *SnapshotResources) virtualDiskHadOwnerReference(ctx context.Context, namespace string, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (bool, error) {
	if IsUnifiedDiskCapture(vdSnapshot) {
		captured, err := CapturedVirtualDisk(ctx, r.client, vdSnapshot)
		if err != nil {
			return false, err
		}
		return hasVirtualMachineOwner(captured), nil
	}

	if vdSnapshot.Status.VolumeSnapshotName == "" {
		return false, nil
	}

	vsKey := types.NamespacedName{Namespace: namespace, Name: vdSnapshot.Status.VolumeSnapshotName}
	vs := &vsv1.VolumeSnapshot{}
	if err := r.client.Get(ctx, vsKey, vs); err != nil {
		return false, fmt.Errorf("failed to get volume snapshot %s: %w", vsKey, err)
	}

	_, ok := vs.Annotations[annotations.AnnVirtualDiskHadOwnerReference]
	return ok, nil
}

func hasVirtualMachineOwner(vd *v1alpha2.VirtualDisk) bool {
	if vd == nil {
		return false
	}
	for _, ownerRef := range vd.OwnerReferences {
		if ownerRef.Kind == v1alpha2.VirtualMachineKind {
			return true
		}
	}
	return false
}

// AddOriginalMetadata copies onto vd the metadata the captured disk had, metadata.name included: a
// restored disk has to come back under the name it was captured as.
//
// Only the captured manifest knows that name for an imported snapshot. spec.virtualDiskName and
// spec.sourceRef are the usual answer, but an import is forbidden to carry either (it captured nothing
// to point at) and the core publishes no status.sourceRef for it, so the object itself records the
// original name nowhere. A name already resolved from the spec is left alone.
func AddOriginalMetadata(ctx context.Context, vd *v1alpha2.VirtualDisk, vdSnapshot *v1alpha2.VirtualDiskSnapshot, client client.Client) error {
	if IsUnifiedDiskCapture(vdSnapshot) {
		// Produced by the state-snapshotter core: there is no CSI VolumeSnapshot to carry the source
		// disk's metadata (disk data is restored via VolumeRestoreRequest instead), but the node's own
		// SnapshotContent holds the VirtualDisk manifest verbatim, so read them straight off it.
		captured, err := CapturedVirtualDisk(ctx, client, vdSnapshot)
		if err != nil {
			return err
		}
		addOriginalMetadataFromCapturedDisk(vd, captured)
		return nil
	}

	vsKey := types.NamespacedName{
		Namespace: vdSnapshot.Namespace,
		Name:      vdSnapshot.Status.VolumeSnapshotName,
	}

	vs, err := object.FetchObject(ctx, vsKey, client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return fmt.Errorf("failed to fetch the volume snapshot %q: %w", vsKey.Name, err)
	}

	if vs == nil {
		return fmt.Errorf("the volume snapshot %q is nil, please report a bug", vsKey.Name)
	}

	return errors.Join(
		setOriginalAnnotations(vd, vs),
		setOriginalLabels(vd, vs),
	)
}

func addOriginalMetadataFromCapturedDisk(vd, captured *v1alpha2.VirtualDisk) {
	if captured == nil {
		return
	}

	if vd.Name == "" {
		vd.Name = captured.Name
	}

	if len(captured.Annotations) > 0 && vd.Annotations == nil {
		vd.Annotations = make(map[string]string, len(captured.Annotations))
	}
	for key, value := range captured.Annotations {
		if _, exists := vd.Annotations[key]; !exists {
			vd.Annotations[key] = value
		}
	}

	if len(captured.Labels) > 0 && vd.Labels == nil {
		vd.Labels = make(map[string]string, len(captured.Labels))
	}
	for key, value := range captured.Labels {
		if _, exists := vd.Labels[key]; !exists {
			vd.Labels[key] = value
		}
	}
}

func setOriginalAnnotations(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) error {
	if vs == nil || vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations] == "" {
		return nil
	}

	var annotationsMap map[string]string
	err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations]), &annotationsMap)
	if err != nil {
		return fmt.Errorf("failed to unmarshal the original annotations: %w", err)
	}

	if vd.Annotations == nil {
		vd.Annotations = make(map[string]string)
	}

	for key, value := range annotationsMap {
		if _, exists := vd.Annotations[key]; !exists {
			vd.Annotations[key] = value
		}
	}

	return nil
}

func setOriginalLabels(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) error {
	if vs == nil || vs.Annotations[annotations.AnnVirtualDiskOriginalLabels] == "" {
		return nil
	}

	var labelsMap map[string]string
	err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalLabels]), &labelsMap)
	if err != nil {
		return fmt.Errorf("failed to unmarshal the original annotations: %w", err)
	}

	if vd.Labels == nil {
		vd.Labels = make(map[string]string)
	}

	for key, value := range labelsMap {
		if _, exists := vd.Labels[key]; !exists {
			vd.Labels[key] = value
		}
	}

	return nil
}
