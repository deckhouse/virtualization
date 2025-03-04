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
	"math"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameStatisticHandler = "StatisticHandler"

func NewStatisticHandler(client client.Client) *StatisticHandler {
	return &StatisticHandler{client: client}
}

type StatisticHandler struct {
	client client.Client
}

func (h *StatisticHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	h.syncStats(current, changed, kvvmi)

	pods, err := s.Pods(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	pod, err := s.Pod(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	h.syncPods(changed, pod, pods)

	h.syncResources(changed, kvvmi, pod)

	return reconcile.Result{}, nil
}

func (h *StatisticHandler) Name() string {
	return nameStatisticHandler
}

func (h *StatisticHandler) syncResources(changed *virtv2.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	pod *corev1.Pod,
) {
	if changed == nil {
		return
	}
	var resources virtv2.ResourcesStatus
	switch pod {
	case nil:
		var (
			cpuKVVMIRequest resource.Quantity
			memorySize      resource.Quantity
			cores           int
			topology        virtv2.Topology
			coreFraction    string
		)
		if kvvmi == nil {
			memorySize = changed.Spec.Memory.Size
			cores = changed.Spec.CPU.Cores
			coreFraction = changed.Spec.CPU.CoreFraction
			sockets, coresPerSocket := vm.CalculateCoresAndSockets(cores)
			topology = virtv2.Topology{CoresPerSocket: coresPerSocket, Sockets: sockets}
		} else {
			cpuKVVMIRequest = kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceCPU]
			memorySize = kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceMemory]

			cores = h.getCoresByKVVMI(kvvmi)
			coreFraction = h.getCoreFractionByKVVMI(kvvmi)
			topology = h.getCurrentTopologyByKVVMI(kvvmi)
		}
		resources = virtv2.ResourcesStatus{
			CPU: virtv2.CPUStatus{
				Cores:          cores,
				CoreFraction:   coreFraction,
				RequestedCores: cpuKVVMIRequest,
				Topology:       topology,
			},
			Memory: virtv2.MemoryStatus{
				Size: memorySize,
			},
		}
	default:
		if kvvmi == nil {
			return
		}
		var ctr corev1.Container
		for _, container := range pod.Spec.Containers {
			if container.Name == "compute" {
				ctr = container
			}
		}

		cpuKVVMIRequest := kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceCPU]
		cpuPODRequest := ctr.Resources.Requests[corev1.ResourceCPU]

		cpuOverhead := cpuPODRequest.DeepCopy()
		cpuOverhead.Sub(cpuKVVMIRequest)

		cores := h.getCoresByKVVMI(kvvmi)
		coreFraction := h.getCoreFractionByKVVMI(kvvmi)
		topology := h.getCurrentTopologyByKVVMI(kvvmi)

		memoryKVVMIRequest := kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceMemory]
		memoryPodRequest := ctr.Resources.Requests[corev1.ResourceMemory]

		memoryOverhead := memoryPodRequest.DeepCopy()
		memoryOverhead.Sub(memoryKVVMIRequest)
		mi := int64(1024 * 1024)
		memoryOverhead = *resource.NewQuantity(int64(math.Ceil(float64(memoryOverhead.Value())/float64(mi)))*mi, resource.BinarySI)

		resources = virtv2.ResourcesStatus{
			CPU: virtv2.CPUStatus{
				Cores:           cores,
				CoreFraction:    coreFraction,
				RequestedCores:  cpuKVVMIRequest,
				RuntimeOverhead: cpuOverhead,
				Topology:        topology,
			},
			Memory: virtv2.MemoryStatus{
				Size:            memoryKVVMIRequest,
				RuntimeOverhead: memoryOverhead,
			},
		}
	}
	changed.Status.Resources = resources
}

func (h *StatisticHandler) getCoresByKVVMI(kvvmi *virtv1.VirtualMachineInstance) int {
	if kvvmi == nil {
		return -1
	}
	cpuKVVMILimit := kvvmi.Spec.Domain.Resources.Limits[corev1.ResourceCPU]
	return int(cpuKVVMILimit.Value())
}

func (h *StatisticHandler) getCoreFractionByKVVMI(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi == nil {
		return ""
	}
	cpuKVVMIRequest := kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceCPU]
	return strconv.Itoa(int(cpuKVVMIRequest.MilliValue())*100/(h.getCoresByKVVMI(kvvmi)*1000)) + "%"
}

func (h *StatisticHandler) getCurrentTopologyByKVVMI(kvvmi *virtv1.VirtualMachineInstance) virtv2.Topology {
	if kvvmi == nil {
		return virtv2.Topology{}
	}

	if kvvmi.Status.CurrentCPUTopology != nil {
		return virtv2.Topology{
			CoresPerSocket: int(kvvmi.Status.CurrentCPUTopology.Cores),
			Sockets:        int(kvvmi.Status.CurrentCPUTopology.Sockets),
		}
	}

	if kvvmi.Spec.Domain.CPU != nil {
		return virtv2.Topology{
			CoresPerSocket: int(kvvmi.Spec.Domain.CPU.Cores),
			Sockets:        int(kvvmi.Spec.Domain.CPU.Sockets),
		}
	}

	cores := h.getCoresByKVVMI(kvvmi)
	sockets, coresPerSocket := vm.CalculateCoresAndSockets(cores)
	return virtv2.Topology{CoresPerSocket: coresPerSocket, Sockets: sockets}
}

func (h *StatisticHandler) syncPods(changed *virtv2.VirtualMachine, pod *corev1.Pod, pods *corev1.PodList) {
	if changed == nil {
		return
	}
	if pods == nil {
		changed.Status.VirtualMachinePods = nil
		return
	}
	virtualMachinePods := make([]virtv2.VirtualMachinePod, len(pods.Items))
	for i, p := range pods.Items {
		active := false
		if pod != nil && p.GetUID() == pod.GetUID() {
			active = true
		}
		virtualMachinePods[i] = virtv2.VirtualMachinePod{
			Name:   p.GetName(),
			Active: active,
		}
	}
	changed.Status.VirtualMachinePods = virtualMachinePods
}

func (h *StatisticHandler) syncStats(current, changed *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if current == nil || changed == nil {
		return
	}
	phaseChanged := current.Status.Phase != changed.Status.Phase

	var stats virtv2.VirtualMachineStats

	if current.Status.Stats != nil {
		stats = *current.Status.Stats.DeepCopy()
	}
	pts := NewPhaseTransitions(stats.PhasesTransitions, current.Status.Phase, changed.Status.Phase)

	stats.PhasesTransitions = pts

	launchTimeDuration := stats.LaunchTimeDuration

	switch changed.Status.Phase {
	case virtv2.MachinePending, virtv2.MachineStopped:
		launchTimeDuration.WaitingForDependencies = nil
		launchTimeDuration.VirtualMachineStarting = nil
		launchTimeDuration.GuestOSAgentStarting = nil
	case virtv2.MachineStarting:
		launchTimeDuration.VirtualMachineStarting = nil
		launchTimeDuration.GuestOSAgentStarting = nil

		if phaseChanged {
			for i := len(pts) - 1; i > 0; i-- {
				pt := pts[i]
				ptPrev := pts[i-1]
				if pt.Phase == virtv2.MachineStarting && ptPrev.Phase == virtv2.MachinePending {
					launchTimeDuration.WaitingForDependencies = &metav1.Duration{Duration: pt.Timestamp.Sub(pts[i-1].Timestamp.Time)}
					break
				}
			}
		}
	case virtv2.MachineRunning:
		if kvvmi != nil && osInfoIsEmpty(kvvmi.Status.GuestOSInfo) {
			launchTimeDuration.GuestOSAgentStarting = nil
		}

		for i := len(pts) - 1; i > 0; i-- {
			pt := pts[i]
			ptPrev := pts[i-1]

			if pt.Phase == virtv2.MachineRunning {
				if phaseChanged && ptPrev.Phase == virtv2.MachineStarting {
					launchTimeDuration.VirtualMachineStarting = &metav1.Duration{Duration: pt.Timestamp.Sub(pts[i-1].Timestamp.Time)}
				}
				if kvvmi != nil && osInfoIsEmpty(current.Status.GuestOSInfo) && !osInfoIsEmpty(kvvmi.Status.GuestOSInfo) && !pt.Timestamp.IsZero() {
					launchTimeDuration.GuestOSAgentStarting = &metav1.Duration{Duration: time.Now().Truncate(time.Second).Sub(pt.Timestamp.Time)}
				}
				break
			}
		}
	}

	stats.LaunchTimeDuration = launchTimeDuration
	changed.Status.Stats = &stats
}

func osInfoIsEmpty(info virtv1.VirtualMachineInstanceGuestOSInfo) bool {
	var emptyOSInfo virtv1.VirtualMachineInstanceGuestOSInfo
	return emptyOSInfo == info
}

func NewPhaseTransitions(phaseTransitions []virtv2.VirtualMachinePhaseTransitionTimestamp, oldPhase, newPhase virtv2.MachinePhase) []virtv2.VirtualMachinePhaseTransitionTimestamp {
	now := metav1.NewTime(time.Now().Truncate(time.Second))

	if oldPhase != newPhase {
		phaseTransitions = append(phaseTransitions, virtv2.VirtualMachinePhaseTransitionTimestamp{
			Phase:     newPhase,
			Timestamp: now,
		})
	}
	if len(phaseTransitions) > 5 {
		return phaseTransitions[len(phaseTransitions)-5:]
	}
	return phaseTransitions
}
