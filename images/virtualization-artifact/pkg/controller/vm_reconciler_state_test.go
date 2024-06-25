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

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var vmControllerLog = logf.Log.WithName("vm-controller-test")

func TestUnmarshalVMStatus(t *testing.T) {
	vmName := types.NamespacedName{
		Namespace: "test-ns",
		Name:      "test-vm",
	}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vmName.Namespace,
			Name:      vmName.Name,
		},
		Spec: v1alpha2.VirtualMachineSpec{
			CPU: v1alpha2.CPUSpec{
				Cores: 2,
			},
			Memory: v1alpha2.MemorySpec{
				Size: "2Gi",
			},
			BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
				{
					Kind: v1alpha2.DiskDevice,
					Name: "test-vmd",
				},
			},
			Disruptions: &v1alpha2.Disruptions{RestartApprovalMode: v1alpha2.Automatic},
		},
		Status: v1alpha2.VirtualMachineStatus{
			Phase:                  v1alpha2.MachineRunning,
			Message:                "",
			RestartAwaitingChanges: nil,
		},
	}

	s := scheme.Scheme
	_ = cdiv1.AddToScheme(s)
	_ = metav1.AddMetaToScheme(s)
	_ = v1alpha2.AddToScheme(s)
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
	state.VM.Changed().Status.RestartAwaitingChanges = statusChanges

	err = cl.Status().Update(ctx, state.VM.Changed())
	require.NoError(t, err, "should update status from changed VM")
}
