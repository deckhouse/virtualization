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

package indexer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	DefaultStorageClass = ""
)

const (
	IndexFieldVMByClass = "spec.virtualMachineClassName"
	IndexFieldVMByVD    = "spec.blockDeviceRefs.VirtualDisk"
	IndexFieldVMByVI    = "spec.blockDeviceRefs.VirtualImage"
	IndexFieldVMByCVI   = "spec.blockDeviceRefs.ClusterVirtualImage"
	IndexFieldVMByNode  = "status.node"

	IndexFieldVDByVDSnapshot  = "vd,spec.DataSource.ObjectRef.Name,.Kind=VirtualDiskSnapshot"
	IndexFieldVIByVDSnapshot  = "vi,spec.DataSource.ObjectRef.Name,.Kind=VirtualDiskSnapshot"
	IndexFieldCVIByVDSnapshot = "cvi,spec.DataSource.ObjectRef.Name,.Kind=VirtualDiskSnapshot"

	IndexFieldVDByStorageClass = "vd.spec.PersistentVolumeClaim.StorageClass"
	IndexFieldVIByStorageClass = "vi.spec.PersistentVolumeClaim.StorageClass"

	IndexFieldVMSnapshotByVM         = "spec.virtualMachineName"
	IndexFieldVMSnapshotByVDSnapshot = "status.virtualDiskSnapshotNames"

	IndexFieldVMRestoreByVMSnapshot = "spec.virtualMachineSnapshotName"

	IndexFieldVMIPByVM      = "status.virtualMachine"
	IndexFieldVMIPByAddress = "spec.staticIP|status.address"

	IndexFieldVMBDAByVM = "spec.virtualMachineName"

	IndexFieldVMMACByVM      = "status.virtualMachine,Kind=VirtualMachineMACAddress"
	IndexFieldVMMACByAddress = "spec.address|status.address"

	IndexFieldVMMACLeaseByVMMAC = "spec.virtualMachineMACAddressRef.Name"

	IndexFieldVMIPLeaseByVMIP = "spec.virtualMachineIPAddressRef"
)

var IndexGetters = []IndexGetter{
	IndexVMByClass,
	IndexVMByVD,
	IndexVMByVI,
	IndexVMByCVI,
	IndexVMByNode,
	IndexVMSnapshotByVM,
	IndexVMSnapshotByVDSnapshot,
	IndexVMRestoreByVMSnapshot,
	IndexVMIPByVM,
	IndexVDByVDSnapshot,
	IndexVDByStorageClass,
	IndexVIByVDSnapshot,
	IndexVIByStorageClass,
	IndexCVIByVDSnapshot,
	IndexVMIPByAddress,
	IndexVMBDAByVM,
	IndexVMMACByVM,
	IndexVMMACByAddress,
	IndexVMMACLeaseByVMMAC,
	IndexVMIPLeaseByVMIP,
}

type IndexGetter func() (obj client.Object, field string, extractValue client.IndexerFunc)

func IndexALL(ctx context.Context, mgr manager.Manager) error {
	for _, fn := range IndexGetters {
		obj, field, indexFunc := fn()
		if err := mgr.GetFieldIndexer().IndexField(ctx, obj, field, indexFunc); err != nil {
			return err
		}
	}
	return nil
}

func IndexVMByClass() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachine{}, IndexFieldVMByClass, func(object client.Object) []string {
		vm, ok := object.(*v1alpha2.VirtualMachine)
		if !ok || vm == nil {
			return nil
		}
		return []string{vm.Spec.VirtualMachineClassName}
	}
}

func IndexVMByVD() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachine{}, IndexFieldVMByVD, func(vm client.Object) []string {
		return getBlockDeviceNamesByKind(vm, v1alpha2.DiskDevice)
	}
}

func IndexVMByVI() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachine{}, IndexFieldVMByVI, func(vm client.Object) []string {
		return getBlockDeviceNamesByKind(vm, v1alpha2.ImageDevice)
	}
}

func IndexVMByCVI() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachine{}, IndexFieldVMByCVI, func(vm client.Object) []string {
		return getBlockDeviceNamesByKind(vm, v1alpha2.ClusterImageDevice)
	}
}

func IndexVMByNode() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualMachine{}, IndexFieldVMByNode, func(object client.Object) []string {
		vm, ok := object.(*v1alpha2.VirtualMachine)
		if !ok || vm == nil || vm.Status.Node == "" {
			return nil
		}
		return []string{vm.Status.Node}
	}
}

func getBlockDeviceNamesByKind(obj client.Object, kind v1alpha2.BlockDeviceKind) []string {
	vm, ok := obj.(*v1alpha2.VirtualMachine)
	if !ok || vm == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var result []string

	for _, ref := range vm.Spec.BlockDeviceRefs {
		if ref.Kind == kind {
			if _, exists := seen[ref.Name]; !exists {
				seen[ref.Name] = struct{}{}
				result = append(result, ref.Name)
			}
		}
	}

	for _, ref := range vm.Status.BlockDeviceRefs {
		if ref.Kind == kind {
			if _, exists := seen[ref.Name]; !exists {
				seen[ref.Name] = struct{}{}
				result = append(result, ref.Name)
			}
		}
	}

	return result
}
