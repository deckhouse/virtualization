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

package vmchange

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestActionRequiredOnCompare(t *testing.T) {
	tests := []struct {
		title       string
		currentSpec string
		desiredSpec string
		assertFn    func(t *testing.T, changes SpecChanges)
	}{
		{
			"restart on cpu.cores change",
			`
cpu:
  cores: 2
`,
			`
cpu:
  cores: 3
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.cores", ChangeReplace),
			),
		},
		{
			"restart on cpu.coreFraction change",
			`
cpu:
  cores: 2
  coreFraction: 60%
`,
			`
cpu:
  cores: 2
  coreFraction: 40%
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.coreFraction", ChangeReplace),
			),
		},
		{
			"restart on cpu section change",
			`
cpu:
  cores: 2
  coreFraction: 60%
`,
			`
cpu:
  cores: 6
  coreFraction: 40%
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu", ChangeReplace),
			),
		},
		{
			"no restart cpu.coreFraction from empty to default value",
			`
cpu:
  cores: 2
`,
			`
cpu:
  cores: 2
  coreFraction: 100%
`,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("cpu.coreFraction", ChangeAdd),
			),
		},
		{
			"no restart cpu.coreFraction from default value to empty",
			`
cpu:
  cores: 2
  coreFraction: 100%
`,
			`
cpu:
  cores: 2
`,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("cpu.coreFraction", ChangeRemove),
			),
		},
		{
			"restart on memory.size change",
			`
memory:
  size: 2Gi
`,
			`
memory:
  size: 1Gi
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		},
		{
			"restart on blockDeviceRefs section add",
			``,
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDeviceRefs", ChangeAdd),
			),
		},
		{
			"restart on blockDeviceRefs section remove",
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			``,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDeviceRefs", ChangeRemove),
			),
		},
		{
			"apply immediate on blockDeviceRefs add disk",
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			`
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeAdd),
			),
		},
		{
			"apply immediate on blockDeviceRefs remove disk",
			`
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeRemove),
			),
		},
		{
			"apply immediate on blockDeviceRefs change order",
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
- kind: VirtualDisk
  name: linux
`,
			`
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeReplace),
				requirePathOperation("blockDeviceRefs.1", ChangeReplace),
			),
		},
		{
			"apply immediate on blockDeviceRefs change order :: bigger",
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
- kind: VirtualDisk
  name: linux
- kind: ClusterVirtualImage
  name: ubuntu
- kind: VirtualDisk
  name: main
- kind: VirtualDisk
  name: data
`,
			// Change order: 12345 -> 25341
			`
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualDisk
  name: data
- kind: ClusterVirtualImage
  name: ubuntu
- kind: VirtualDisk
  name: main
- kind: VirtualImage
  name: linux
`,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeReplace),
				requirePathOperation("blockDeviceRefs.1", ChangeReplace),
				requirePathOperation("blockDeviceRefs.4", ChangeReplace),
			),
		},
		{
			"restart on provisioning add",
			`
`,
			`
provisioning:
  type: UserData
  userData: |
    #cloudinit
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeAdd),
			),
		},
		{
			"restart on provisioning remove",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-secret
`,
			"",
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeRemove),
			),
		},
		{
			"restart on provisioning type change",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-secret
`,
			`
provisioning:
  type: UserData
  userData: |
    #cloudinit
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeReplace),
			),
		},
		{
			"restart on provisioning secretref name change",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-secret
`,
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: provisioning-secret
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.userDataRef.name", ChangeReplace),
			),
		},
		{
			"restart on enableParavirtualization change true to false",
			`
enableParavirtualization: true
`,
			`
enableParavirtualization: false
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("enableParavirtualization", ChangeReplace),
			),
		},
		{
			"restart on enableParavirtualization change false to true",
			`
enableParavirtualization: false
`,
			`
enableParavirtualization: true
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("enableParavirtualization", ChangeReplace),
			),
		},
	}

	for _, tt := range tests {
		var changes SpecChanges
		t.Run(tt.title, func(t *testing.T) {
			currentSpec := loadVMSpec(t, tt.currentSpec)
			desiredSpec := loadVMSpec(t, tt.desiredSpec)

			changes = CompareVMSpecs(currentSpec, desiredSpec)

			defer func() {
				if t.Failed() {
					t.Log(changesToYAML(changes))
				}
			}()

			tt.assertFn(t, changes)
		})
	}
}

func loadVMSpec(t *testing.T, inYAML string) *v1alpha2.VirtualMachineSpec {
	t.Helper()
	var spec v1alpha2.VirtualMachineSpec
	err := yaml.Unmarshal([]byte(inYAML), &spec)
	require.NoError(t, err, "Should load vm spec from '%s'", inYAML)
	return &spec
}

func assertChanges(asserts ...func(t *testing.T, changes SpecChanges)) func(t *testing.T, changes SpecChanges) {
	return func(t *testing.T, changes SpecChanges) {
		t.Helper()

		for _, fn := range asserts {
			fn(t, changes)
		}
	}
}

func actionRequired(actionType ActionType) func(t *testing.T, changes SpecChanges) {
	return func(t *testing.T, changes SpecChanges) {
		t.Helper()
		require.NotNil(t, changes, "changes should not be nil, as test requires action: %s", actionType)
		require.Equal(t, actionType, changes.ActionType(), "action required %s, got %s, empty=%v, changes=%+v", actionType, changes.ActionType(), changes.IsEmpty(), changes)
	}
}

func requirePathOperation(path string, operation ChangeOperation) func(t *testing.T, changes SpecChanges) {
	return func(t *testing.T, changes SpecChanges) {
		t.Helper()

		hasPathOperation := false
		var pathOperation ChangeOperation
		var change FieldChange

		for _, fieldChange := range changes.GetAll() {
			if fieldChange.Path == path {
				pathOperation = fieldChange.Operation
				change = fieldChange
				hasPathOperation = true
				break
			}
		}

		require.True(t, hasPathOperation, "changes should contain FieldChange path=%s operation=%s", path, operation)
		require.Equal(t, operation, pathOperation, "change for path=%s should be %s, got %s: %+v", path, operation, pathOperation, change)
	}
}

func changesToYAML(changes SpecChanges) string {
	status := map[string]interface{}{
		"status": map[string]interface{}{
			"pendingChanges": json.RawMessage(changes.ToJSON()),
		},
	}

	res, _ := yaml.Marshal(status)
	return string(res)
}
