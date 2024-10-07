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
	IndexFieldVMByClass = "spec.virtualMachineClassName"
	IndexFieldVMByVD    = "spec.blockDeviceRefs.VirtualDisk"
	IndexFieldVMByVI    = "spec.blockDeviceRefs.VirtualImage"
	IndexFieldVMByCVI   = "spec.blockDeviceRefs.ClusterVirtualImage"

	IndexFieldVMIPLeaseByVMIP = "spec.virtualMachineIPAddressRef.Name"

	IndexFieldVDByVDSnapshot = "spec.DataSource.ObjectRef.Name,.Kind=VirtualDiskSnapshot"

	IndexFieldVMSnapshotByVM         = "spec.virtualMachineName"
	IndexFieldVMSnapshotByVDSnapshot = "status.virtualDiskSnapshotNames"

	IndexFieldVMRestoreByVMSnapshot = "spec.virtualMachineSnapshotName"

	IndexFieldVMIPByVM      = "status.virtualMachine"
	IndexFieldVMIPByAddress = "spec.staticIP|status.address"
)

type indexFunc func(ctx context.Context, mgr manager.Manager) error

func IndexALL(ctx context.Context, mgr manager.Manager) error {
	for _, fn := range []indexFunc{
		IndexVMByClass,
		IndexVMByVD,
		IndexVMByVI,
		IndexVMByCVI,
		IndexVMIPLeaseByVMIP,
		IndexVDByVDSnapshot,
		IndexVMSnapshotByVM,
		IndexVMSnapshotByVDSnapshot,
		IndexVMIPByVM,
		IndexVMIPByAddress,
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

func IndexVMIPLeaseByVMIP(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &virtv2.VirtualMachineIPAddressLease{}, IndexFieldVMIPLeaseByVMIP, func(object client.Object) []string {
		lease, ok := object.(*virtv2.VirtualMachineIPAddressLease)
		if !ok || lease == nil {
			return nil
		}
		return []string{lease.Spec.VirtualMachineIPAddressRef.Name}
	})
}
