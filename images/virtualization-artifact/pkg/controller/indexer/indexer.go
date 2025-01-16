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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	DefaultStorageClass = ""
)

const (
	IndexFieldVMByClass = "spec.virtualMachineClassName"
	IndexFieldVMByVD    = "spec.blockDeviceRefs.VirtualDisk"
	IndexFieldVMByVI    = "spec.blockDeviceRefs.VirtualImage"
	IndexFieldVMByCVI   = "spec.blockDeviceRefs.ClusterVirtualImage"

	IndexFieldVMIPLeaseByVMIP   = "spec.virtualMachineIPAddressRef.Name"
	IndexFieldVMMACLeaseByVMMAC = "spec.virtualMachineMACAddressRef.Name"

	IndexFieldVDByVDSnapshot = "spec.DataSource.ObjectRef.Name,.Kind=VirtualDiskSnapshot"

	IndexFieldVDByStorageClass = "VD.spec.PersistentVolumeClaim.StorageClass"
	IndexFieldVIByStorageClass = "VI.spec.PersistentVolumeClaim.StorageClass"

	IndexFieldVMSnapshotByVM         = "spec.virtualMachineName"
	IndexFieldVMSnapshotByVDSnapshot = "status.virtualDiskSnapshotNames"

	IndexFieldVMRestoreByVMSnapshot = "spec.virtualMachineSnapshotName"

	IndexFieldVMIPByVM      = "status.virtualMachine,Kind=VirtualMachineIPAddress"
	IndexFieldVMMACByVM     = "status.virtualMachine,Kind=VirtualMachineMACAddress"
	IndexFieldVMIPByAddress = "spec.staticIP|status.address"

	IndexFieldVMBDAByVM = "spec.virtualMachineName"
)

type indexFunc func(ctx context.Context, mgr manager.Manager) error

func IndexALL(ctx context.Context, mgr manager.Manager) error {
	for _, fn := range []indexFunc{
		IndexVMByClass,
		IndexVMByVD,
		IndexVMByVI,
		IndexVMByCVI,
		IndexVMIPLeaseByVMIP,
		IndexVMMACLeaseByVMMAC,
		IndexVDByVDSnapshot,
		IndexVMSnapshotByVM,
		IndexVMSnapshotByVDSnapshot,
		IndexVMRestoreByVMSnapshot,
		IndexVMIPByVM,
		IndexVMMACByVM,
		IndexVDByStorageClass,
		IndexVIByStorageClass,
		IndexVMIPByAddress,
		IndexVMBDAByVM,
	} {
		if err := fn(ctx, mgr); err != nil {
			return err
		}
	}
	return nil
}

func IndexVMByClass(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachine{}, IndexFieldVMByClass, func(object client.Object) []string {
		vm, ok := object.(*virtv2.VirtualMachine)
		if !ok || vm == nil {
			return nil
		}
		return []string{vm.Spec.VirtualMachineClassName}
	})
}

func IndexVMByVD(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachine{}, IndexFieldVMByVD, func(object client.Object) []string {
		return getBlockDeviceNamesByKind(object, virtv2.DiskDevice)
	})
}

func IndexVMByVI(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachine{}, IndexFieldVMByVI, func(object client.Object) []string {
		return getBlockDeviceNamesByKind(object, virtv2.ImageDevice)
	})
}

func IndexVMByCVI(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachine{}, IndexFieldVMByCVI, func(object client.Object) []string {
		return getBlockDeviceNamesByKind(object, virtv2.ClusterImageDevice)
	})
}

func getBlockDeviceNamesByKind(obj client.Object, kind virtv2.BlockDeviceKind) []string {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok || vm == nil {
		return nil
	}
	var res []string
	for _, bdr := range vm.Spec.BlockDeviceRefs {
		if bdr.Kind != kind {
			continue
		}
		res = append(res, bdr.Name)
	}
	return res
}
