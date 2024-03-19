package kvbuilder

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func Test_kvvm_compare_resources(t *testing.T) {
	vm := &virtv2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-vm",
		},
		Spec: virtv2.VirtualMachineSpec{
			RunPolicy: virtv2.AlwaysOnPolicy,
			OsType:    virtv2.GenericOs,
			CPU: virtv2.CPUSpec{
				Cores: 2,
			},
			Memory: virtv2.MemorySpec{
				Size: "2Gi",
			},
		},
		Status: virtv2.VirtualMachineStatus{},
	}

	curr := NewEmptyKVVM(types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}, KVVMOptions{})
	err := ApplyVirtualMachineSpec(curr, vm, nil, nil, nil, nil, "")
	require.NoError(t, err, "ApplyVirtualMachineSpec should not fail")
	vm.Spec.Memory.Size = "5Gi"

	next := NewEmptyKVVM(types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}, KVVMOptions{})
	err = ApplyVirtualMachineSpec(next, vm, nil, nil, nil, nil, "")
	require.NoError(t, err, "ApplyVirtualMachineSpec should not fail")

	actions, err := CompareKVVM(curr, next)

	require.NoError(t, err, "CompareKVVM should not fail")
	require.NotEmpty(t, actions, "Should return action on memory change")

	require.Equal(t, ActionRestart, actions.ActionType())
}
