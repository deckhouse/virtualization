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

package restore

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk/transform"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Transformer is our github.com/deckhouse/state-snapshotter/pkg/snapshotsdk/transform.Transformer
// implementation: it points a restored VirtualDisk at its own VirtualDiskSnapshot via
// spec.dataSource.objectRef, the same field shape the existing (unmodified) VirtualDisk controller
// already knows how to provision from — see vdsnapshot.ensureVolumeSnapshotBridge for the other half of
// that contract (VirtualDiskSnapshot.status.volumeSnapshotName).
type Transformer struct{}

var _ transform.Transformer = Transformer{}

// CoveredPVCNames is always empty: our manifest captures are VirtualMachine/VirtualDisk objects only,
// never PersistentVolumeClaims directly.
func (Transformer) CoveredPVCNames(_ *transform.RestoreNode, _ []unstructured.Unstructured) map[string]struct{} {
	return map[string]struct{}{}
}

// TransformObject sets a VirtualDisk's spec.dataSource to its own owning VirtualDiskSnapshot node.
func (Transformer) TransformObject(node *transform.RestoreNode, obj *unstructured.Unstructured, _ []transform.NodeResult) (bool, error) {
	if !isVirtualDisk(*obj) {
		return false, nil
	}
	if node == nil || node.SnapshotRef.Kind != v1alpha2.VirtualDiskSnapshotKind {
		return false, nil
	}
	dataSource := map[string]interface{}{
		"type": string(v1alpha2.DataSourceTypeObjectRef),
		"objectRef": map[string]interface{}{
			"kind": string(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot),
			"name": node.SnapshotRef.Name,
		},
	}
	if err := unstructured.SetNestedMap(obj.Object, dataSource, "spec", "dataSource"); err != nil {
		return false, fmt.Errorf("set VirtualDisk %s spec.dataSource: %w", obj.GetName(), err)
	}
	return true, nil
}

func isVirtualDisk(obj unstructured.Unstructured) bool {
	return obj.GetKind() == v1alpha2.VirtualDiskKind && obj.GetAPIVersion() == v1alpha2.SchemeGroupVersion.String()
}

func applyKeepIPAddress(obj *unstructured.Unstructured, keep v1alpha2.KeepIPAddress) error {
	if keep != v1alpha2.KeepIPAddressAlways || !isVirtualMachineIPAddress(*obj) {
		return nil
	}

	typ, _, err := unstructured.NestedString(obj.Object, "spec", "type")
	if err != nil {
		return fmt.Errorf("read VirtualMachineIPAddress %s spec.type: %w", obj.GetName(), err)
	}
	if typ != string(v1alpha2.VirtualMachineIPAddressTypeAuto) {
		return nil
	}

	address, _, err := unstructured.NestedString(obj.Object, "status", "address")
	if err != nil {
		return fmt.Errorf("read VirtualMachineIPAddress %s status.address: %w", obj.GetName(), err)
	}
	if address == "" {
		return fmt.Errorf(
			"cannot honor keepIPAddress=Always: captured VirtualMachineIPAddress %q has an Auto type and no allocated address",
			obj.GetName(),
		)
	}

	if err := unstructured.SetNestedField(obj.Object, string(v1alpha2.VirtualMachineIPAddressTypeStatic), "spec", "type"); err != nil {
		return fmt.Errorf("set VirtualMachineIPAddress %s spec.type: %w", obj.GetName(), err)
	}
	if err := unstructured.SetNestedField(obj.Object, address, "spec", "staticIP"); err != nil {
		return fmt.Errorf("set VirtualMachineIPAddress %s spec.staticIP: %w", obj.GetName(), err)
	}
	return nil
}

func isVirtualMachineIPAddress(obj unstructured.Unstructured) bool {
	return obj.GetKind() == v1alpha2.VirtualMachineIPAddressKind && obj.GetAPIVersion() == v1alpha2.SchemeGroupVersion.String()
}

// linkCapturedIPAddress points the restored VirtualMachine at the VirtualMachineIPAddress captured alongside it.
func linkCapturedIPAddress(objs []unstructured.Unstructured) error {
	var name string
	for i := range objs {
		if isVirtualMachineIPAddress(objs[i]) {
			name = objs[i].GetName()
			break
		}
	}
	if name == "" {
		return nil
	}

	for i := range objs {
		if !isVirtualMachine(objs[i]) {
			continue
		}
		if err := unstructured.SetNestedField(objs[i].Object, name, "spec", "virtualMachineIPAddressName"); err != nil {
			return fmt.Errorf("set VirtualMachine %s spec.virtualMachineIPAddressName: %w", objs[i].GetName(), err)
		}
	}
	return nil
}

func isVirtualMachine(obj unstructured.Unstructured) bool {
	return obj.GetKind() == v1alpha2.VirtualMachineKind && obj.GetAPIVersion() == v1alpha2.SchemeGroupVersion.String()
}

func applyCapturedMACAddresses(objs []unstructured.Unstructured) error {
	captured := make(map[string]struct{})
	for i := range objs {
		if !isVirtualMachineMACAddress(objs[i]) {
			continue
		}
		if err := pinCapturedMACAddress(&objs[i]); err != nil {
			return err
		}
		captured[objs[i].GetName()] = struct{}{}
	}

	for i := range objs {
		if !isVirtualMachine(objs[i]) {
			continue
		}
		if err := linkCapturedMACAddresses(&objs[i], captured); err != nil {
			return err
		}
	}
	return nil
}

func pinCapturedMACAddress(obj *unstructured.Unstructured) error {
	specAddress, _, err := unstructured.NestedString(obj.Object, "spec", "address")
	if err != nil {
		return fmt.Errorf("read VirtualMachineMACAddress %s spec.address: %w", obj.GetName(), err)
	}
	if specAddress != "" {
		return nil
	}

	address, _, err := unstructured.NestedString(obj.Object, "status", "address")
	if err != nil {
		return fmt.Errorf("read VirtualMachineMACAddress %s status.address: %w", obj.GetName(), err)
	}
	if address == "" {
		return fmt.Errorf(
			"captured VirtualMachineMACAddress %q has no address in either spec or status: the restored virtual machine would come back with a different MAC",
			obj.GetName(),
		)
	}

	if err := unstructured.SetNestedField(obj.Object, address, "spec", "address"); err != nil {
		return fmt.Errorf("set VirtualMachineMACAddress %s spec.address: %w", obj.GetName(), err)
	}
	return nil
}

func linkCapturedMACAddresses(vm *unstructured.Unstructured, captured map[string]struct{}) error {
	specNetworks, found, err := unstructured.NestedSlice(vm.Object, "spec", "networks")
	if err != nil {
		return fmt.Errorf("read VirtualMachine %s spec.networks: %w", vm.GetName(), err)
	}
	if !found || len(specNetworks) == 0 {
		return nil
	}

	statusNetworks, _, err := unstructured.NestedSlice(vm.Object, "status", "networks")
	if err != nil {
		return fmt.Errorf("read VirtualMachine %s status.networks: %w", vm.GetName(), err)
	}
	if len(captured) > 0 && len(statusNetworks) < len(specNetworks) {
		return fmt.Errorf(
			"captured VirtualMachine %q declares %d networks in spec but only %d in status: cannot restore the MAC address order",
			vm.GetName(), len(specNetworks), len(statusNetworks),
		)
	}

	for i := range specNetworks {
		specNetwork, ok := specNetworks[i].(map[string]interface{})
		if !ok {
			return fmt.Errorf("captured VirtualMachine %q spec.networks[%d] is not an object", vm.GetName(), i)
		}

		name := ""
		if i < len(statusNetworks) {
			statusNetwork, ok := statusNetworks[i].(map[string]interface{})
			if !ok {
				return fmt.Errorf("captured VirtualMachine %q status.networks[%d] is not an object", vm.GetName(), i)
			}
			name, _ = statusNetwork["virtualMachineMACAddressName"].(string)
		}

		if _, ok := captured[name]; !ok {
			delete(specNetwork, "virtualMachineMACAddressName")
			continue
		}
		specNetwork["virtualMachineMACAddressName"] = name
	}

	if err := unstructured.SetNestedSlice(vm.Object, specNetworks, "spec", "networks"); err != nil {
		return fmt.Errorf("set VirtualMachine %s spec.networks: %w", vm.GetName(), err)
	}
	return nil
}

func isVirtualMachineMACAddress(obj unstructured.Unstructured) bool {
	return obj.GetKind() == v1alpha2.VirtualMachineMACAddressKind && obj.GetAPIVersion() == v1alpha2.SchemeGroupVersion.String()
}
