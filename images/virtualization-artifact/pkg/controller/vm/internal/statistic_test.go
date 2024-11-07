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

package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vmtemplate "github.com/deckhouse/virtualization-controller/pkg/controller/template/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func oldTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05+07:00")
	_, offset := time.Now().Zone()
	diff := time.Duration(offset) * time.Second
	return t.Add(-diff).In(time.Local)
}

func TestStatisticHandler(t *testing.T) {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		virtv2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
	} {
		err := f(scheme)
		if err != nil {
			t.Fatalf("failed to add scheme: %v", err)
		}
	}

	namespacedName := types.NamespacedName{Name: "test-vm", Namespace: "test-namespace"}

	for _, test := range []struct {
		name          string
		getObjects    func() []client.Object
		mutateChanged func(vm *virtv2.VirtualMachine)
		expect        func(vm *virtv2.VirtualMachine) error
	}{
		{
			name: "test1: virtualmachine running, if nil statistics",
			getObjects: func() []client.Object {
				return []client.Object{
					createPod(namespacedName),
					createVM(namespacedName, virtv2.MachinePending, nil, virtv1.VirtualMachineInstanceGuestOSInfo{}),
					createKVVMI(namespacedName, virtv1.Running, virtv1.VirtualMachineInstanceGuestOSInfo{Name: "test"}),
				}
			},
			mutateChanged: func(vm *virtv2.VirtualMachine) {
				if vm != nil {
					vm.Status.Phase = virtv2.MachineRunning
				}
			},
			expect: func(vm *virtv2.VirtualMachine) error {
				if vm == nil || vm.Status.Stats == nil {
					return fmt.Errorf("expected vm or stats to not be nil")
				}
				expectPhasesTransitions := []virtv2.MachinePhase{
					virtv2.MachinePending,
					virtv2.MachineStarting,
					virtv2.MachineRunning,
				}
				if err := checkPhasesTransitions(t, expectPhasesTransitions, vm); err != nil {
					return err
				}
				require.NotNil(t, vm.Status.Stats)
				require.NotNil(t, vm.Status.Stats.LaunchTimeDuration.WaitingForDependencies)
				require.NotNil(t, vm.Status.Stats.LaunchTimeDuration.VirtualMachineStarting)
				require.NotNil(t, vm.Status.Stats.LaunchTimeDuration.GuestOSAgentStarting)
				return nil
			},
		},
		{
			name: "test2: virtualmachine running, statistic should no change",
			getObjects: func() []client.Object {
				info := virtv1.VirtualMachineInstanceGuestOSInfo{Name: "test"}
				return []client.Object{
					createPod(namespacedName),
					createVM(namespacedName, virtv2.MachineRunning, createStatisticNoChange(), info),
					createKVVMI(namespacedName, virtv1.Running, info),
				}
			},
			mutateChanged: func(vm *virtv2.VirtualMachine) {},
			expect: func(vm *virtv2.VirtualMachine) error {
				if vm == nil || vm.Status.Stats == nil {
					return fmt.Errorf("expected vm or stats to not be nil")
				}
				stats := createStatisticNoChange()
				require.Equal(t, stats.PhasesTransitions, vm.Status.Stats.PhasesTransitions)
				require.Equal(t, stats.LaunchTimeDuration, vm.Status.Stats.LaunchTimeDuration)
				return nil
			},
		},
		{
			name: "test3: .Stats.LaunchTimeDuration.WaitingForDependencies was should changed",
			getObjects: func() []client.Object {
				info := virtv1.VirtualMachineInstanceGuestOSInfo{}
				return []client.Object{
					createPod(namespacedName),
					createVM(namespacedName,
						virtv2.MachinePending,
						&virtv2.VirtualMachineStats{
							PhasesTransitions: []virtv2.VirtualMachinePhaseTransitionTimestamp{
								{
									Phase:     virtv2.MachinePending,
									Timestamp: metav1.NewTime(oldTime()),
								},
								{
									Phase:     virtv2.MachineStarting,
									Timestamp: metav1.NewTime(oldTime().Add(10 * time.Second)),
								},
							},
							LaunchTimeDuration: virtv2.VirtualMachineLaunchTimeDuration{
								WaitingForDependencies: &metav1.Duration{Duration: 10 * time.Second},
							},
						},
						info),
					createKVVMI(namespacedName, virtv1.Scheduling, info),
				}
			},
			mutateChanged: func(vm *virtv2.VirtualMachine) {
				if vm == nil {
					return
				}
				vm.Status.Phase = virtv2.MachineStarting
			},
			expect: func(vm *virtv2.VirtualMachine) error {
				if vm == nil || vm.Status.Stats == nil {
					return fmt.Errorf("expected vm or stats to not be nil")
				}
				expectPhasesTransitions := []virtv2.MachinePhase{
					virtv2.MachinePending,
					virtv2.MachineStarting,
				}
				if err := checkPhasesTransitions(t, expectPhasesTransitions, vm); err != nil {
					return err
				}
				require.NotNil(t, vm.Status.Stats)
				wfd := vm.Status.Stats.LaunchTimeDuration.WaitingForDependencies
				require.NotNil(t, wfd)
				require.Greater(t, wfd.Duration, 10*time.Second)

				return nil
			},
		},
		{
			name: "test4: check generated .status.resources",
			getObjects: func() []client.Object {
				return []client.Object{
					createPod(namespacedName),
					createVM(namespacedName, virtv2.MachineRunning, nil, virtv1.VirtualMachineInstanceGuestOSInfo{}),
					createKVVMI(namespacedName, virtv1.Running, virtv1.VirtualMachineInstanceGuestOSInfo{Name: "test"}),
				}
			},
			mutateChanged: func(vm *virtv2.VirtualMachine) {},
			expect: func(vm *virtv2.VirtualMachine) error {
				require.NotNil(t, vm)
				res := vm.Status.Resources
				require.Equal(t, res.CPU.Cores, 1)
				require.Equal(t, res.CPU.CoreFraction, "50%")
				require.Equal(t, res.CPU.RequestedCores.MilliValue(), int64(500))
				require.Equal(t, res.CPU.RuntimeOverhead.MilliValue(), int64(0))
				require.Equal(t, res.Memory.Size.Value(), int64(536870912))
				require.Equal(t, res.Memory.RuntimeOverhead.Value(), int64(254803968))
				return nil
			},
		},
	} {
		t.Log("Start test", test.name)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(test.getObjects()...).Build()
		vm := service.NewResource(namespacedName, fakeClient, factory, statusGetter)
		if err := vm.Fetch(context.Background()); err != nil {
			t.Fatalf("failed to fetch resource: %v", err)
		}
		s := state.New(fakeClient, vm)
		test.mutateChanged(s.VirtualMachine().Changed())
		handler := NewStatisticHandler(fakeClient)
		_, err := handler.Handle(context.Background(), s)
		if err != nil {
			t.Fatalf("failed to sync stats: %v", err)
		}
		if err = test.expect(s.VirtualMachine().Changed()); err != nil {
			t.Fatalf("test %q failed: %v", test.name, err)
		}
	}
}

func checkPhasesTransitions(t *testing.T, expectPhasesTransitions []virtv2.MachinePhase, vm *virtv2.VirtualMachine) error {
	if vm == nil || vm.Status.Stats == nil {
		return fmt.Errorf("expected vm or stats to not be nil")
	}
	var pts []virtv2.MachinePhase
	timestamp := oldTime().Add(-24 * time.Hour)
	for _, pt := range vm.Status.Stats.PhasesTransitions {
		if pt.Timestamp.After(timestamp) {
			timestamp = pt.Timestamp.Time
		} else {
			return fmt.Errorf("wrong sort phases")
		}
		pts = append(pts, pt.Phase)
	}
	require.Equal(t, expectPhasesTransitions, pts)
	return nil
}

func createStatisticNoChange() *virtv2.VirtualMachineStats {
	return &virtv2.VirtualMachineStats{
		PhasesTransitions: []virtv2.VirtualMachinePhaseTransitionTimestamp{
			{
				Phase:     virtv2.MachinePending,
				Timestamp: metav1.Time{Time: oldTime()},
			},
			{
				Phase:     virtv2.MachineStarting,
				Timestamp: metav1.Time{Time: oldTime().Add(1 * time.Second)},
			},
			{
				Phase:     virtv2.MachineRunning,
				Timestamp: metav1.Time{Time: oldTime().Add(2 * time.Second)},
			},
			{
				Phase:     virtv2.MachineStopping,
				Timestamp: metav1.Time{Time: oldTime().Add(3 * time.Second)},
			},
		},
		LaunchTimeDuration: virtv2.VirtualMachineLaunchTimeDuration{
			WaitingForDependencies: &metav1.Duration{Duration: 1 * time.Second},
			VirtualMachineStarting: &metav1.Duration{Duration: 1 * time.Second},
			GuestOSAgentStarting:   &metav1.Duration{Duration: 1 * time.Second},
		},
	}
}

func createPod(vmKey types.NamespacedName) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("virt-launcher-%s", vmKey.Name),
			Namespace: vmKey.Namespace,
			Labels: map[string]string{
				virtv1.VirtualMachineNameLabel: vmKey.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "compute",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("755Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("755Mi"),
						},
					},
				},
			},
		},
	}
}

func createVM22(key types.NamespacedName,
	phase virtv2.MachinePhase,
	stats *virtv2.VirtualMachineStats,
	guestOSInfo virtv1.VirtualMachineInstanceGuestOSInfo) {

	options := []vmtemplate.Option{
		vmtemplate.WithCPUSpec(virtv2.CPUSpec{
			Cores:        1,
			CoreFraction: "50%",
		}),
		vmtemplate.WithMemorySpec(virtv2.MemorySpec{
			Size: resource.MustParse("512Mi"),
		}),
		vmtemplate.WithStatus(virtv2.VirtualMachineStatus{
			Phase:       phase,
			Stats:       stats,
			GuestOSInfo: guestOSInfo,
		}),
	}
	vmtemplate.New(key).WithOptions(options...)
}

func createVM(key types.NamespacedName,
	phase virtv2.MachinePhase,
	stats *virtv2.VirtualMachineStats,
	guestOSInfo virtv1.VirtualMachineInstanceGuestOSInfo,
) *virtv2.VirtualMachine {
	return &virtv2.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			Kind:       virtv2.VirtualMachineKind,
			APIVersion: virtv2.GroupVersionResource(virtv2.VirtualMachineKind).GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: virtv2.VirtualMachineSpec{
			CPU: virtv2.CPUSpec{
				Cores:        1,
				CoreFraction: "50%",
			},
			Memory: virtv2.MemorySpec{
				Size: resource.MustParse("512Mi"),
			},
		},
		Status: virtv2.VirtualMachineStatus{
			Phase:       phase,
			Stats:       stats,
			GuestOSInfo: guestOSInfo,
		},
	}
}

func createKVVMI(key types.NamespacedName,
	phase virtv1.VirtualMachineInstancePhase,
	guestOSInfo virtv1.VirtualMachineInstanceGuestOSInfo,
) *virtv1.VirtualMachineInstance {
	return &virtv1.VirtualMachineInstance{
		TypeMeta: metav1.TypeMeta{
			Kind:       virtv1.VirtualMachineGroupVersionKind.Kind,
			APIVersion: virtv1.VirtualMachineGroupVersionKind.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: virtv1.VirtualMachineInstanceSpec{
			Domain: virtv1.DomainSpec{
				Resources: virtv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		},
		Status: virtv1.VirtualMachineInstanceStatus{
			Phase:       phase,
			GuestOSInfo: guestOSInfo,
		},
	}
}

func factory() *virtv2.VirtualMachine {
	return &virtv2.VirtualMachine{}
}

func statusGetter(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus {
	return obj.Status
}
