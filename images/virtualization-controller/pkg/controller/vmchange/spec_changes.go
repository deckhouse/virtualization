package vmchange

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"sigs.k8s.io/yaml"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

const NoChanges = "NoChanges"

// SpecChanges calculates a set of changes between 2 VM specs and actions required to apply changes to VM.
//
// Last used spec applied to the KVVM is considered "current".
// Actual VM spec is considered "desired".
//
// Examples:
// current spec:
//
//	disruptions:
//	  approvalMode: Manual
//
// desired spec:
//
//	disruptions:
//	  approvalMode: Automatic
//
// status:
//
//	pendingChanges:
//	- operation: replace
//	  path: disruptions.approvalMode
//	  currentValue: Manual
//	  desiredValue: Automatic
//
// Remove disruptions:
// current spec:
//
//	cpu: ...
//	disruptions
//	  approvalMode: Manual
//	memory: ...
//
// desired:
//
//	cpu: ...
//	memory: ...
//
// status:
//
//	pendingChanges:
//	- operation: remove
//	  path: disruptions
//	  currentValue:
//	    approvalMode: Manual
//
// Change cpu settings:
// current spec:
//
//	cpu:
//	  cores: 2
//	  coreFraction: 25%
//
// desired:
//
//	cpu:
//	  cores: 6
//	  coreFraction: 100%
//
// status:
//
//	pendingChanges:
//	- operation: replace
//	  path: cpu
//	  currentValue:
//	    cores: 2
//	    coreFraction: 25%
//	  desiredValue:
//	    cores: 6
//	    coreFraction: 100%
//
// Change block device.
// current spec:
//
//	blockDevices:
//	- type: ClusterVirtualMachineImage
//	  clusterVirtualMachineImage: {name: linux-ubuntu}
//
// desired spec:
//
//	blockDevices:
//	- type: VirtualMachineImage
//	  virtualMachineImage: {name: jammy-ubuntu}
//
// status:
//
//	pendingChanges:
//	- op: replace
//	  path: blockDevices.0
//	  currentValue:
//	    type: ClusterVirtualMachineImage
//	    clusterVirtualMachineImage: {name: linux-ubuntu}
//	  desiredValue:
//	    type: VirtualMachineImage
//	    virtualMachineImage: {name: jammy-ubuntu}
//
// Remove, add block devices.
// current spec:
//
//	blockDevices:
//	- type: ClusterVirtualMachineImage
//	  clusterVirtualMachineImage: {name: linux-ubuntu}
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk}
//
// desired spec:
//
//	blockDevices:
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk}
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk-data}
//
// status:
//
//	pendingChanges:
//	- operation: remove
//	  path: blockDevices.0
//	  currentValue:
//	    type: ClusterVirtualMachineImage
//	    clusterVirtualMachineImage: {name: linux-ubuntu}
//	- operation: add
//	  path: blockDevices.1
//	  desiredValue:
//	    type: VirtualMachineDisk
//	    virtualMachineDisk: {name: vm-disk-data}
//
// Multiple operations: remove, add, change order.
// Operations are compacted by index:
// current spec:
//
//	blockDevices:
//	- type: ClusterVirtualMachineImage
//	  clusterVirtualMachineImage: {name: linux-ubuntu}
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk}
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk-big}
//	- type: VirtualMachineImage
//	  virtualMachineImage: {name: jammy-ubuntu}
//
// desired spec:
//
//	blockDevices:
//	- type: VirtualMachineImage
//	  virtualMachineImage: {name: jammy-ubuntu}
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk-2}
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk-big}  <-- the only disk saves its index.
//	- type: VirtualMachineDisk
//	  virtualMachineDisk: {name: vm-disk}
//
// status:
//
//	pendingChanges:
//	- operation: replace
//	  path: blockDevices.0
//	  currentValue:
//	    type: ClusterVirtualMachineImage
//	    clusterVirtualMachineImage: {name: linux-ubuntu}
//	  desiredValue:
//	    type: VirtualMachineImage
//	    virtualMachineImage: {name: jammy-ubuntu}
//	- operation: replace
//	  path: blockDevices.1
//	  currentValue:
//	    type: VirtualMachineDisk
//	    virtualMachineDisk: {name: vm-disk}
//	  desiredValue:
//	    type: VirtualMachineDisk
//	    virtualMachineDisk: {name: vm-disk-2}
//	- operation: replace
//	  path: blockDevices.3
//	  currentValue:
//	    type: VirtualMachineImage
//	    virtualMachineImage: {name: jammy-ubuntu}
//	  desiredValue:
//	    type: VirtualMachineDisk
//	    virtualMachineDisk: {name: vm-disk}
type SpecChanges struct {
	changes []FieldChange
}

func (s *SpecChanges) IsEmpty() bool {
	return s == nil || len(s.changes) == 0
}

func (s *SpecChanges) GetAll() []FieldChange {
	return s.changes
}

func (s *SpecChanges) GetPendingChanges() []virtv2.FieldChangeOperation {
	res := make([]virtv2.FieldChangeOperation, 0, len(s.changes))
	for i := range s.changes {
		res = append(res, s.changes[i])
	}
	return res
}

func (s *SpecChanges) Add(changes ...FieldChange) {
	if s.changes == nil {
		s.changes = make([]FieldChange, 0)
	}
	s.changes = append(s.changes, changes...)
}

// ActionType returns the most dangerous action type:
// None < ApplyImmediate < SubresourceSignal < Restart
func (s *SpecChanges) ActionType() ActionType {
	if s.IsEmpty() {
		return ActionNone
	}

	// Types from most dangerous to least dangerous.
	typesInOrder := []ActionType{
		ActionRestart,
		ActionSubresourceSignal,
		ActionApplyImmediate,
	}

	for _, typ := range typesInOrder {
		for _, fieldChange := range s.changes {
			if fieldChange.ActionRequired == typ {
				return typ
			}
		}
	}

	return ActionNone
}

func (s *SpecChanges) IsDisruptive() bool {
	return s.ActionType() == ActionRestart
}

func (s *SpecChanges) ChangeID() string {
	if len(s.changes) == 0 {
		return NoChanges
	}

	// Sort by path.
	sort.SliceStable(s.changes, func(i, j int) bool {
		return s.changes[i].Path < s.changes[j].Path
	})

	hasher := sha256.New()
	hasher.Write([]byte(s.ToJSON()))

	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func (s *SpecChanges) ToJSON() string {
	// Sort by path.
	sort.SliceStable(s.changes, func(i, j int) bool {
		return s.changes[i].Path < s.changes[j].Path
	})

	b, _ := json.Marshal(s.changes)
	return string(b)
}

func (s *SpecChanges) ToYAML() string {
	// Sort by path.
	sort.SliceStable(s.changes, func(i, j int) bool {
		return s.changes[i].Path < s.changes[j].Path
	})

	b, _ := yaml.Marshal(s.changes)
	return string(b)
}
