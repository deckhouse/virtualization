package vmchange

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
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
			"restart on blockDevices section add",
			``,
			`
blockDevices:
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDevices", ChangeAdd),
			),
		},
		{
			"restart on blockDevices section remove",
			`
blockDevices:
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			``,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDevices", ChangeRemove),
			),
		},
		{
			"restart on blockDevices add disk",
			`
blockDevices:
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			`
blockDevices:
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: linux
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDevices.0", ChangeAdd),
			),
		},
		{
			"restart on blockDevices remove disk",
			`
blockDevices:
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: linux
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			`
blockDevices:
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDevices.0", ChangeRemove),
			),
		},
		{
			"restart on blockDevices change order",
			`
blockDevices:
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: linux
`,
			`
blockDevices:
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: linux
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDevices.0", ChangeReplace),
				requirePathOperation("blockDevices.1", ChangeReplace),
			),
		},
		{
			"restart on blockDevices change order :: bigger",
			`
blockDevices:
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: linux
- type: ClusterVirtualMachineImage
  virtualMachineImage:
    name: ubuntu
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: main
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: data
`,
			// Change order: 12345 -> 25341
			`
blockDevices:
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: linux
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: data
- type: ClusterVirtualMachineImage
  virtualMachineImage:
    name: ubuntu
- type: VirtualMachineDisk
  virtualMachineDisk:
    name: main
- type: VirtualMachineImage
  virtualMachineImage:
    name: linux
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDevices.0", ChangeReplace),
				requirePathOperation("blockDevices.1", ChangeReplace),
				requirePathOperation("blockDevices.4", ChangeReplace),
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
  type: UserDataSecret
  userDataSecretRef:
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
  type: UserDataSecret
  userDataSecretRef:
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
  type: UserDataSecret
  userDataSecretRef:
    name: cloud-init-secret
`,
			`
provisioning:
  type: UserDataSecret
  userDataSecretRef:
    name: provisioning-secret
`,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.userDataSecretRef.name", ChangeReplace),
			),
		},
	}

	for _, tt := range tests {
		var changes SpecChanges
		t.Run(tt.title, func(t *testing.T) {
			currentSpec := loadVMSpec(t, tt.currentSpec)
			desiredSpec := loadVMSpec(t, tt.desiredSpec)

			changes = CompareSpecs(currentSpec, desiredSpec)

			defer func() {
				if t.Failed() {
					t.Log(changesToYAML(changes))
				}
			}()

			tt.assertFn(t, changes)
		})
	}
}

func loadVMSpec(t *testing.T, inYAML string) *virtv2.VirtualMachineSpec {
	t.Helper()
	var spec virtv2.VirtualMachineSpec
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
