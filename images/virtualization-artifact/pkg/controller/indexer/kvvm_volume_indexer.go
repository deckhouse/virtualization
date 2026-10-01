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

package indexer

import (
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// IndexKVVMIByVolume indexes an instance by its volumes: in the spec from the moment KubeVirt takes the
// attach request, and in the status until the volume is detached from the guest.
func IndexKVVMIByVolume() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv1.VirtualMachineInstance{}, IndexFieldKVVMIByVolume, func(object client.Object) []string {
		kvvmi, ok := object.(*virtv1.VirtualMachineInstance)
		if !ok || kvvmi == nil {
			return nil
		}
		names := make([]string, 0, len(kvvmi.Spec.Volumes)+len(kvvmi.Status.VolumeStatus))
		for _, v := range kvvmi.Spec.Volumes {
			names = append(names, v.Name)
		}
		for _, vs := range kvvmi.Status.VolumeStatus {
			names = append(names, vs.Name)
		}
		return names
	}
}

// IndexKVVMByAddVolumeRequest indexes an internal virtual machine by the volumes of its pending attach requests.
func IndexKVVMByAddVolumeRequest() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &virtv1.VirtualMachine{}, IndexFieldKVVMByAddVolumeRequest, func(object client.Object) []string {
		kvvm, ok := object.(*virtv1.VirtualMachine)
		if !ok || kvvm == nil {
			return nil
		}
		var names []string
		for _, vr := range kvvm.Status.VolumeRequests {
			if vr.AddVolumeOptions != nil {
				names = append(names, vr.AddVolumeOptions.Name)
			}
		}
		return names
	}
}

// IndexVDByVolumeName indexes a disk by the name of its KubeVirt volume: a shortened volume name cannot be
// mapped back to the disk.
func IndexVDByVolumeName() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &v1alpha2.VirtualDisk{}, IndexFieldVDByVolumeName, func(object client.Object) []string {
		vd, ok := object.(*v1alpha2.VirtualDisk)
		if !ok || vd == nil {
			return nil
		}
		return []string{kvbuilder.GenerateVDDiskName(vd.Name)}
	}
}
