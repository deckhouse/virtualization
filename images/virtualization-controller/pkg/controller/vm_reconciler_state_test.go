package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
)

func TestUnmarshalVMStatus(t *testing.T) {
	vmName := types.NamespacedName{
		Namespace: "test-ns",
		Name:      "test-vm",
	}
	vm := &v2alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vmName.Namespace,
			Name:      vmName.Name,
		},
		Spec: v2alpha1.VirtualMachineSpec{
			CPU: v2alpha1.CPUSpec{
				Cores: 2,
			},
			Memory: v2alpha1.MemorySpec{
				Size: "2Gi",
			},
			BlockDevices: []v2alpha1.BlockDeviceSpec{
				{
					Type:               v2alpha1.DiskDevice,
					VirtualMachineDisk: &v2alpha1.DiskDeviceSpec{Name: "test-vmd"},
				},
			},
			Disruptions: &v2alpha1.Disruptions{ApprovalMode: v2alpha1.Automatic},
		},
		Status: v2alpha1.VirtualMachineStatus{
			Phase:          v2alpha1.MachineRunning,
			Message:        "",
			ChangeID:       "",
			PendingChanges: nil,
		},
	}

	s := scheme.Scheme
	_ = cdiv1.AddToScheme(s)
	_ = metav1.AddMetaToScheme(s)
	_ = v2alpha1.AddToScheme(s)
	_ = virtv1.AddToScheme(s)

	builder := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(vm).
		WithRuntimeObjects(vm)

	cl := builder.Build()
	vmControllerLog = logf.Log.WithName("vm-controller-test")
	req := reconcile.Request{NamespacedName: vmName}
	ctx := context.Background()

	state := NewVMReconcilerState(
		vmName,
		vmControllerLog,
		cl,
		nil,
	)
	err := state.Reload(ctx, req, vmControllerLog, cl)
	require.NoError(t, err, "should reload successfully")

	require.False(t, state.VM.IsEmpty(), "loaded VM should not be empty")

	require.Equal(t, vmName.Name, state.VM.Current().Name, "should load current VM")
	require.NotNil(t, state.VM.Changed(), "should load changed VM")

	var changes vmchange.SpecChanges
	changes.Add(vmchange.FieldChange{
		Operation:      "replace",
		Path:           "spec",
		CurrentValue:   true,
		DesiredValue:   false,
		ActionRequired: vmchange.ActionRestart,
	})

	statusChanges, err := changes.ConvertPendingChanges()
	require.NoError(t, err, "should convert pending changes")
	state.VM.Changed().Status.PendingChanges = statusChanges

	err = cl.Status().Update(ctx, state.VM.Changed())
	require.NoError(t, err, "should update status from changed VM")
}
