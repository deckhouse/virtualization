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

package vd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func GetCurrentlyMountedVMName(vd *v1alpha2.VirtualDisk) string {
	for _, attachedVM := range vd.Status.AttachedToVirtualMachines {
		if attachedVM.Mounted {
			return attachedVM.Name
		}
	}
	return ""
}

func IsMigrating(vd *v1alpha2.VirtualDisk) bool {
	return vd != nil && !vd.Status.MigrationState.StartTimestamp.IsZero() && vd.Status.MigrationState.EndTimestamp.IsZero()
}

// VolumeMigrationEnabled returns true if volume migration is enabled or if the volume is currently migrating
// If the volume migrating but the feature gate was turned off, we should complete the migration
func VolumeMigrationEnabled(gate featuregate.FeatureGate, vd *v1alpha2.VirtualDisk) bool {
	if gate.Enabled(featuregates.VolumeMigration) {
		return true
	}
	if IsMigrating(vd) {
		slog.Info("VolumeMigration is disabled, but the volume is already migrating. Complete the migration.", slog.String("vd.name", vd.Name), slog.String("vd.namespace", vd.Namespace))
		return true
	}
	return false
}

func StorageClassChanged(vd *v1alpha2.VirtualDisk) bool {
	if vd == nil {
		return false
	}

	specSc := vd.Spec.PersistentVolumeClaim.StorageClass
	if specSc == nil {
		return false
	}

	statusSc := vd.Status.StorageClassName
	if *specSc == statusSc {
		return false
	}

	return *specSc != "" && statusSc != ""
}

type VirtualDiskStorageClassResolver interface {
	GetModuleStorageClass(ctx context.Context) (*storagev1.StorageClass, error)
	GetDefaultStorageClass(ctx context.Context) (*storagev1.StorageClass, error)
}

// ResolveStorageClassName resolves storage class name for a VirtualDisk
// with the same precedence as VD handlers:
// 1. vd.Status.StorageClassName
// 2. vd.Spec.PersistentVolumeClaim.StorageClass
// 3. module default storage class (if resolver is provided)
// 4. cluster default storage class (if resolver is provided)
func ResolveStorageClassName(ctx context.Context, vd *v1alpha2.VirtualDisk, resolver VirtualDiskStorageClassResolver) (string, error) {
	if vd == nil {
		return "", nil
	}

	if vd.Status.StorageClassName != "" {
		return vd.Status.StorageClassName, nil
	}

	if vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "" {
		return *vd.Spec.PersistentVolumeClaim.StorageClass, nil
	}

	if resolver == nil {
		return "", nil
	}

	moduleStorageClass, err := resolver.GetModuleStorageClass(ctx)
	if err != nil {
		return "", err
	}
	if moduleStorageClass != nil {
		return moduleStorageClass.Name, nil
	}

	defaultStorageClass, err := resolver.GetDefaultStorageClass(ctx)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) {
		return "", err
	}
	if defaultStorageClass != nil {
		return defaultStorageClass.Name, nil
	}

	return "", fmt.Errorf("storage class for VirtualDisk %q cannot be determined", vd.Name)
}

// GetNodePlacement resolves the node and tolerations import helpers must use
// so they can run wherever the consuming VirtualMachine is scheduled.
func GetNodePlacement(ctx context.Context, c client.Client, vd *v1alpha2.VirtualDisk) (*provisioner.NodePlacement, error) {
	if len(vd.Status.AttachedToVirtualMachines) != 1 {
		return nil, nil
	}

	vmKey := types.NamespacedName{Name: vd.Status.AttachedToVirtualMachines[0].Name, Namespace: vd.Namespace}
	vm, err := object.FetchObject(ctx, vmKey, c, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("unable to get the virtual machine %s: %w", vmKey, err)
	}

	if vm == nil {
		return nil, nil
	}

	var nodePlacement provisioner.NodePlacement
	// The node the VM is scheduled on (empty until the VM's virt-launcher pod is
	// scheduled). Import helpers (prime PVC, importer pod) are pinned here so a
	// WaitForFirstConsumer node-local volume is provisioned on the VM's node.
	nodePlacement.Node = vm.Status.Node
	nodePlacement.Tolerations = append(nodePlacement.Tolerations, vm.Spec.Tolerations...)

	vmClassKey := types.NamespacedName{Name: vm.Spec.VirtualMachineClassName}
	vmClass, err := object.FetchObject(ctx, vmClassKey, c, &v1alpha2.VirtualMachineClass{})
	if err != nil {
		return nil, fmt.Errorf("unable to get the virtual machine class %s: %w", vmClassKey, err)
	}

	if vmClass == nil {
		return &nodePlacement, nil
	}

	nodePlacement.Tolerations = append(nodePlacement.Tolerations, vmClass.Spec.Tolerations...)

	return &nodePlacement, nil
}

// ValidateVirtualImageStorageClassProvisionerCompatibility forbids provisioning a
// VirtualDisk from a PVC-backed VirtualImage that lives on a storage class backed
// by a different CSI driver: the PVC-to-PVC copy cannot cross the driver boundary.
func ValidateVirtualImageStorageClassProvisionerCompatibility(ctx context.Context, vd *v1alpha2.VirtualDisk, client client.Client) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef {
		return nil
	}

	if vd.Spec.DataSource.ObjectRef == nil || vd.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualDiskObjectRefKindVirtualImage {
		return nil
	}

	vi, err := object.FetchObject(ctx, types.NamespacedName{Namespace: vd.Namespace, Name: vd.Spec.DataSource.ObjectRef.Name}, client, &v1alpha2.VirtualImage{})
	if err != nil {
		return err
	}

	if vi == nil || vi.Status.Phase != v1alpha2.ImageReady || vi.Spec.Storage == v1alpha2.StorageContainerRegistry {
		return nil
	}

	vdSc, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, client, &storagev1.StorageClass{})
	if err != nil {
		return fmt.Errorf("get virtual disk storage class %q: %w", vd.Status.StorageClassName, err)
	}
	if vdSc == nil {
		return fmt.Errorf("virtual disk storage class %q was not found", vd.Status.StorageClassName)
	}

	viSc, err := object.FetchObject(ctx, types.NamespacedName{Name: vi.Status.StorageClassName}, client, &storagev1.StorageClass{})
	if err != nil {
		return fmt.Errorf("get virtual image storage class %q: %w", vi.Status.StorageClassName, err)
	}
	if viSc == nil {
		return fmt.Errorf("virtual image storage class %q was not found", vi.Status.StorageClassName)
	}

	if vdSc.Provisioner != viSc.Provisioner {
		return fmt.Errorf(
			"virtual disk storage class %q provisioner does not match virtual image storage class %q provisioner: source type with different provisioners is not supported yet",
			vd.Status.StorageClassName,
			vi.Status.StorageClassName,
		)
	}

	return nil
}
