/*
Copyright 2025 Flant JSC

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

package powerstate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

func TestRestartVM(t *testing.T) {
	const (
		vmName                = "test-vm"
		vmNamespace           = "test-namespace"
		nodeName              = "worker-01"
		kvvmiUID    types.UID = "test-kvvmi-uid"
		podUID      types.UID = "test-pod-uid"
	)
	key := types.NamespacedName{
		Name:      vmName,
		Namespace: vmNamespace,
	}

	newClient := func() (client.WithWatch, error) {
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachine",
				APIVersion: virtv1.SchemeGroupVersion.String(),
			},
		}
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
				UID:       kvvmiUID,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineInstance",
				APIVersion: virtv1.SchemeGroupVersion.String(),
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				NodeName: nodeName,
				ActivePods: map[types.UID]string{
					podUID: vmName,
				},
			},
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
				Labels: map[string]string{
					virtv1.AppLabel:       "virt-launcher",
					virtv1.CreatedByLabel: string(kvvmiUID),
				},
				UID: podUID,
			},
			Spec: corev1.PodSpec{
				NodeName: nodeName,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
		}

		return testutil.NewFakeClientWithObjects(kvvm, kvvmi, pod)
	}

	for _, tt := range []struct {
		name  string
		force bool
	}{
		{
			name:  "Should patched with stop,start requests",
			force: false,
		},
		{
			name:  "Should patched with stop,start requests and delete pod",
			force: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := newClient()
			require.NoError(t, err)

			oldKVVM := &virtv1.VirtualMachine{}
			err = c.Get(context.Background(), key, oldKVVM)
			require.NoError(t, err)

			oldKVVMI := &virtv1.VirtualMachineInstance{}
			err = c.Get(context.Background(), key, oldKVVMI)
			require.NoError(t, err)

			err = RestartVM(context.Background(), c, oldKVVM, oldKVVMI, tt.force)
			require.NoError(t, err)

			newKVVM := &virtv1.VirtualMachine{}
			err = c.Get(context.Background(), key, newKVVM)
			require.NoError(t, err)

			require.NotEmpty(t, newKVVM.Status.StateChangeRequests)
			require.Len(t, newKVVM.Status.StateChangeRequests, 2)
			require.Equal(t, virtv1.StopRequest, newKVVM.Status.StateChangeRequests[0].Action)
			require.NotNil(t, newKVVM.Status.StateChangeRequests[0].UID)
			require.Equal(t, kvvmiUID, *newKVVM.Status.StateChangeRequests[0].UID)
			require.Equal(t, virtv1.StartRequest, newKVVM.Status.StateChangeRequests[1].Action)

			pod := &corev1.Pod{}
			err = c.Get(context.Background(), key, pod)
			if tt.force {
				require.Error(t, err)
				require.True(t, k8serrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
			}
		})
	}
}
