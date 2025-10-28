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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	restorer       *SecretRestorer
	restorerSecret *corev1.Secret
	vmSnapshot     *v1alpha2.VirtualMachineSnapshot
	objectHandlers []ObjectHandler
	statuses       []v1alpha2.SnapshotResourceStatus
	mode           v1alpha2.VMOPRestoreMode
	kind           v1alpha2.VMOPType
}

func NewSnapshotResources(client client.Client, kind v1alpha2.VMOPType, mode v1alpha2.VMOPRestoreMode, restorerSecret *corev1.Secret, vmSnapshot *v1alpha2.VirtualMachineSnapshot, uuid string) SnapshotResources {
	return SnapshotResources{
		mode:           mode,
		kind:           kind,
		uuid:           uuid,
		client:         client,
		restorer:       NewSecretRestorer(client),
		vmSnapshot:     vmSnapshot,
		restorerSecret: restorerSecret,
	}
}

func (r *SnapshotResources) Prepare(ctx context.Context) error {
	provisioner, err := r.restorer.RestoreProvisioner(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	vm, err := r.restorer.RestoreVirtualMachine(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	vmip, err := r.restorer.RestoreVirtualMachineIPAddress(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	if vmip != nil && r.kind == v1alpha2.VMOPTypeRestore {
		vm.Spec.VirtualMachineIPAddress = vmip.Name
	} else {
		vm.Spec.VirtualMachineIPAddress = ""
	}

	vmmacs, err := r.restorer.RestoreVirtualMachineMACAddresses(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	macAddressOrder, err := r.restorer.RestoreMACAddressOrder(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	vds, err := getVirtualDisks(ctx, r.client, r.vmSnapshot)
	if err != nil {
		return err
	}

	vmbdas, err := r.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	if len(vmmacs) > 0 && r.kind == v1alpha2.VMOPTypeRestore {
		macAddressNamesByAddress := make(map[string]string)
		for _, vmmac := range vmmacs {
			r.objectHandlers = append(r.objectHandlers, restorer.NewVirtualMachineMACAddressHandler(r.client, vmmac, r.uuid))
			macAddressNamesByAddress[vmmac.Status.Address] = vmmac.Name
		}

		for i := range vm.Spec.Networks {
			ns := &vm.Spec.Networks[i]
			if ns.Type == v1alpha2.NetworksTypeMain {
				continue
			}

			ns.VirtualMachineMACAddressName = macAddressNamesByAddress[macAddressOrder[i-1]]
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

	r.objectHandlers = append(r.objectHandlers, restorer.NewVirtualMachineHandler(r.client, *vm, string(r.vmSnapshot.UID), r.mode))

	return nil
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

	if r.mode == v1alpha2.VMOPRestoreModeDryRun {
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

	return r.statuses, nil
}

var DryRunIgnoredErrors = []error{
	common.ErrVMMaintenanceCondNotFound,
	common.ErrVMNotInMaintenance,
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

func shouldIgnoreError(mode v1alpha2.VMOPRestoreMode, err error) bool {
	switch mode {
	case v1alpha2.VMOPRestoreModeDryRun:
		for _, e := range DryRunIgnoredErrors {
			if errors.Is(err, e) {
				return true
			}
		}
	case v1alpha2.VMOPRestoreModeBestEffort:
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

func getVirtualDisks(ctx context.Context, client client.Client, vmSnapshot *v1alpha2.VirtualMachineSnapshot) ([]*v1alpha2.VirtualDisk, error) {
	vds := make([]*v1alpha2.VirtualDisk, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch the virtual disk snapshot %q: %w", vdSnapshotKey.Name, err)
		}

		if vdSnapshot == nil {
			return nil, fmt.Errorf("the virtual disk snapshot %q %w", vdSnapshotName, common.ErrVirtualDiskSnapshotNotFound)
		}

		vd := v1alpha2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualDiskKind,
				APIVersion: v1alpha2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdSnapshot.Spec.VirtualDiskName,
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
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{Name: vmSnapshot.Spec.VirtualMachineName, Mounted: true},
				},
			},
		}

		vds = append(vds, &vd)
	}

	return vds, nil
}

func (r *SnapshotResources) GetObjectHandlers() []ObjectHandler {
	return r.objectHandlers
}
