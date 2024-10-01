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
	"sort"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
	switch {
	case pod == nil:
		var (
			cpuKVVMIRequest resource.Quantity
			memorySize      resource.Quantity
			cores           int
			coreFraction    string
		)
		if kvvmi == nil {
			memorySize = changed.Spec.Memory.Size
			cores = changed.Spec.CPU.Cores
			coreFraction = changed.Spec.CPU.CoreFraction
		} else {
			cpuKVVMIRequest = kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceCPU]
			memorySize = kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceMemory]

			cores = h.getCoresByKVVMI(kvvmi)
			coreFraction = h.getCoreFractionByKVVMI(kvvmi)
		}
		resources = virtv2.ResourcesStatus{
			CPU: virtv2.CPUStatus{
				Cores:          cores,
				CoreFraction:   coreFraction,
				RequestedCores: cpuKVVMIRequest,
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
	var stats virtv2.VirtualMachineStats

	if current.Status.Stats != nil {
		stats = *current.Status.Stats.DeepCopy()
	}
	pts := NewPhaseTransitions(stats.PhasesTransitions, current.Status.Phase, changed.Status.Phase)
	pts.Sort()
	stats.PhasesTransitions = pts.Items

	launchTimeDuration := stats.LaunchTimeDuration

	switch changed.Status.Phase {
	case virtv2.MachinePending:
		launchTimeDuration.WaitingForDependencies = nil
		launchTimeDuration.VirtualMachineStarting = nil
		launchTimeDuration.GuestOSAgentStarting = nil
	case virtv2.MachineStarting:
		launchTimeDuration.VirtualMachineStarting = nil
		launchTimeDuration.GuestOSAgentStarting = nil
	case virtv2.MachineRunning:
		if kvvmi != nil && osInfoIsEmpty(kvvmi.Status.GuestOSInfo) {
			launchTimeDuration.GuestOSAgentStarting = nil
		}
	}

	for i, pt := range pts.Items {
		switch pt.Phase {
		case virtv2.MachineStarting:
			if i > 0 && pts.Items[i-1].Phase == phasePreviousPhase[pt.Phase] {
				launchTimeDuration.WaitingForDependencies = &metav1.Duration{Duration: pt.Timestamp.Sub(pts.Items[i-1].Timestamp.Time)}
			}
		case virtv2.MachineRunning:
			if i > 0 && pts.Items[i-1].Phase == phasePreviousPhase[pt.Phase] {
				launchTimeDuration.VirtualMachineStarting = &metav1.Duration{Duration: pt.Timestamp.Sub(pts.Items[i-1].Timestamp.Time)}
			}
			if kvvmi != nil && osInfoIsEmpty(current.Status.GuestOSInfo) && !osInfoIsEmpty(kvvmi.Status.GuestOSInfo) && !pt.Timestamp.IsZero() {
				launchTimeDuration.GuestOSAgentStarting = &metav1.Duration{Duration: time.Now().Truncate(time.Second).Sub(pt.Timestamp.Time)}
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

var phasePreviousPhase = map[virtv2.MachinePhase]virtv2.MachinePhase{
	virtv2.MachineRunning:  virtv2.MachineStarting,
	virtv2.MachineStarting: virtv2.MachinePending,
	virtv2.MachineStopped:  virtv2.MachineStopping,
}

type PhaseTransitions struct {
	Items []virtv2.VirtualMachinePhaseTransitionTimestamp
}

func NewPhaseTransitions(phaseTransitions []virtv2.VirtualMachinePhaseTransitionTimestamp, oldPhase, newPhase virtv2.MachinePhase) PhaseTransitions {
	now := metav1.NewTime(time.Now().Truncate(time.Second))

	phasesTransitionsMap := make(map[virtv2.MachinePhase]virtv2.VirtualMachinePhaseTransitionTimestamp, len(phaseTransitions))
	for _, pt := range phaseTransitions {
		phasesTransitionsMap[pt.Phase] = pt
	}
	if _, found := phasesTransitionsMap[newPhase]; !found || oldPhase != newPhase {
		phasesTransitionsMap[newPhase] = virtv2.VirtualMachinePhaseTransitionTimestamp{
			Phase:     newPhase,
			Timestamp: now,
		}
	}
	p := newPhase
	t := now.Add(-1 * time.Second)
	// Since we are setting up phases based on kvvm, we may skip some of them.
	// But we need to know some timestamps to generate statistics.
	// Add the missing phases.
	for {
		if previousPhase, found := phasePreviousPhase[p]; found {
			// if p > .... > previousPhase || not found
			if previousPt, found := phasesTransitionsMap[previousPhase]; !found {
				phasesTransitionsMap[previousPhase] = virtv2.VirtualMachinePhaseTransitionTimestamp{
					Phase:     previousPhase,
					Timestamp: metav1.NewTime(t),
				}
				t = t.Add(-1 * time.Second)
			} else {
				// if p > .... > previousPhase ; then do p > previousPhase > ...
				currentPt := phasesTransitionsMap[p]
				for _, pt := range phasesTransitionsMap {
					if pt.Phase == currentPt.Phase || pt.Phase == previousPt.Phase {
						continue
					}
					if pt.Timestamp.After(previousPt.Timestamp.Time) && currentPt.Timestamp.After(pt.Timestamp.Time) {
						phasesTransitionsMap[previousPhase] = virtv2.VirtualMachinePhaseTransitionTimestamp{
							Phase:     previousPhase,
							Timestamp: metav1.NewTime(t),
						}
						t = t.Add(-1 * time.Second)
						break
					}
				}
			}
			p = previousPhase
			continue
		}
		break
	}
	phasesTransitionsSlice := make([]virtv2.VirtualMachinePhaseTransitionTimestamp, len(phasesTransitionsMap))
	i := 0
	for _, p := range phasesTransitionsMap {
		phasesTransitionsSlice[i] = p
		i++
	}
	return PhaseTransitions{Items: phasesTransitionsSlice}
}

func (pt *PhaseTransitions) Sort() {
	sort.Sort(pt)
}

func (pt *PhaseTransitions) Len() int {
	return len(pt.Items)
}

func (pt *PhaseTransitions) Less(i, j int) bool {
	return pt.Items[j].Timestamp.After(pt.Items[i].Timestamp.Time)
}

func (pt *PhaseTransitions) Swap(i, j int) {
	pt.Items[i], pt.Items[j] = pt.Items[j], pt.Items[i]
}
