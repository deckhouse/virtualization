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
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const (
	nameStatisticHandler = "StatisticHandler"
	// TODO: Remove this fallback after 2026-10-29.
	lastStartTimePhaseTransitionMaxDiff = 10 * time.Minute
)

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

	if err := h.syncResources(changed, kvvmi, pod); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (h *StatisticHandler) Name() string {
	return nameStatisticHandler
}

func (h *StatisticHandler) syncResources(changed *v1alpha2.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	pod *corev1.Pod,
) error {
	if changed == nil {
		return nil
	}
	var resources v1alpha2.ResourcesStatus
	switch pod {
	case nil:
		var (
			cpuKVVMIRequest *resource.Quantity
			memorySize      resource.Quantity
			topology        v1alpha2.Topology
			coreFraction    string
		)
		if kvvmi == nil {
			memorySize = changed.Spec.Memory.Size
			sockets, coresPerSocket, _ := vm.CalculateCoresAndSockets(changed.Spec.CPU.Cores)
			topology = v1alpha2.Topology{CoresPerSocket: coresPerSocket, Sockets: sockets}
			coreFraction = changed.Spec.CPU.CoreFraction
		} else {
			var err error
			cpuKVVMIRequest, err = h.getCoresRequestedByKVVMI(kvvmi)
			if err != nil {
				return err
			}
			memorySize = kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceMemory]

			coreFraction = h.getCoreFractionByKVVMI(kvvmi)
			topology = h.getCurrentTopologyByKVVMI(kvvmi)
		}
		resources = v1alpha2.ResourcesStatus{
			CPU: v1alpha2.CPUStatus{
				Cores:        topology.CoresPerSocket * topology.Sockets,
				CoreFraction: coreFraction,
				Topology:     topology,
			},
			Memory: v1alpha2.MemoryStatus{
				Size: memorySize,
			},
		}
		if cpuKVVMIRequest != nil {
			resources.CPU.RequestedCores = *cpuKVVMIRequest
		}

	default:
		if kvvmi == nil {
			return nil
		}
		var ctr corev1.Container
		for _, container := range pod.Spec.Containers {
			if vm.IsComputeContainer(container.Name) {
				ctr = container
			}
		}

		coreFraction := h.getCoreFractionByKVVMI(kvvmi)
		topology := h.getCurrentTopologyByKVVMI(kvvmi)
		cores := topology.CoresPerSocket * topology.Sockets

		cpuFractionRequests, err := h.getCoresRequestedByKVVMI(kvvmi)
		if err != nil {
			return fmt.Errorf("get core fraction by kvvmi: %w", err)
		}
		cpuPODRequest := ctr.Resources.Requests[corev1.ResourceCPU]

		cpuOverhead := cpuPODRequest.DeepCopy()
		cpuOverhead.Sub(*cpuFractionRequests)
		if cpuOverhead.Value() < 0 {
			cpuOverhead.Set(0)
		}

		memoryKVVMIRequest := kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceMemory]
		memoryPodRequest := ctr.Resources.Requests[corev1.ResourceMemory]

		memoryOverhead := memoryPodRequest.DeepCopy()
		memoryOverhead.Sub(memoryKVVMIRequest)
		mi := int64(1024 * 1024)
		memoryOverhead = *resource.NewQuantity(int64(math.Ceil(float64(memoryOverhead.Value())/float64(mi)))*mi, resource.BinarySI)
		if memoryOverhead.Value() < 0 {
			memoryOverhead.Set(0)
		}

		resources = v1alpha2.ResourcesStatus{
			CPU: v1alpha2.CPUStatus{
				Cores:           cores,
				CoreFraction:    coreFraction,
				RequestedCores:  *cpuFractionRequests,
				RuntimeOverhead: cpuOverhead,
				Topology:        topology,
			},
			Memory: v1alpha2.MemoryStatus{
				Size:            memoryKVVMIRequest,
				RuntimeOverhead: memoryOverhead,
			},
		}
	}
	changed.Status.Resources = resources
	return nil
}

// getCoresByKVVMI
// TODO refactor: no need to get cores from limits after enabling CPU hotplug, kvvmi.Spec.Domain.CPU should be enough.
func (h *StatisticHandler) getCoresByKVVMI(kvvmi *virtv1.VirtualMachineInstance) int {
	if kvvmi == nil {
		return -1
	}

	cpuKVVMILimit, hasLimits := kvvmi.Spec.Domain.Resources.Limits[corev1.ResourceCPU]
	if hasLimits {
		return int(cpuKVVMILimit.Value())
	}

	return 1
}

func (h *StatisticHandler) getCoreFractionByKVVMI(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi == nil {
		return ""
	}
	// Fraction is stored in annotation after enabling CPU hotplug.
	cpuFractionStr, hasAnno := kvvmi.Annotations[kvbuilder.CPUResourcesRequestsFractionAnnotation]
	if hasAnno {
		return cpuFractionStr + "%"
	}
	// Also support previous implementation: calculate from requests and limits values.
	cpuKVVMIRequest := kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceCPU]
	return strconv.Itoa(int(cpuKVVMIRequest.MilliValue())*100/(h.getCoresByKVVMI(kvvmi)*1000)) + "%"
}

func (h *StatisticHandler) getCoresRequestedByKVVMI(kvvmi *virtv1.VirtualMachineInstance) (*resource.Quantity, error) {
	if kvvmi == nil {
		return nil, nil
	}
	// Fraction is stored in annotation after enabling CPU hotplug.
	cpuFractionStr, hasAnno := kvvmi.Annotations[kvbuilder.CPUResourcesRequestsFractionAnnotation]
	if hasAnno {
		if kvvmi.Spec.Domain.CPU == nil {
			return nil, fmt.Errorf("enabled dynamic cores with annotation %s, but missing spec.domain.cpu", kvbuilder.CPUResourcesRequestsFractionAnnotation)
		}
		cores := kvvmi.Spec.Domain.CPU.Cores * kvvmi.Spec.Domain.CPU.Sockets

		cpuFraction, err := strconv.Atoi(cpuFractionStr)
		if err != nil {
			return nil, err
		}

		if cpuFraction <= 0 || cpuFraction > 100 {
			cpuFraction = 100
		}
		if cpuFraction == 100 {
			return resource.NewQuantity(int64(cores), resource.DecimalSI), nil
		}

		// Use multiplier to calculate fraction of millis.
		requested := cores * 1000
		// Round up, to always return integer number of millis.
		value := int64(math.Ceil(float64(cpuFraction) * (float64(requested)) / 100))
		return resource.NewMilliQuantity(value, resource.DecimalSI), nil
	}

	// Also support previous implementation: return cpu requests if set.
	if reqCPU, hasCPURequests := kvvmi.Spec.Domain.Resources.Requests[corev1.ResourceCPU]; hasCPURequests {
		return &reqCPU, nil
	}

	return nil, nil
}

func (h *StatisticHandler) getCurrentTopologyByKVVMI(kvvmi *virtv1.VirtualMachineInstance) v1alpha2.Topology {
	if kvvmi == nil {
		return v1alpha2.Topology{}
	}

	cores := -1
	sockets := -1

	if kvvmi.Status.CurrentCPUTopology != nil {
		cores = int(kvvmi.Status.CurrentCPUTopology.Cores)
		sockets = int(kvvmi.Status.CurrentCPUTopology.Sockets)
	}

	if kvvmi.Spec.Domain.CPU != nil {
		cores = int(kvvmi.Spec.Domain.CPU.Cores)
		sockets = int(kvvmi.Spec.Domain.CPU.Sockets)
	}

	if _, isDynamicCores := kvvmi.Annotations[kvbuilder.VCPUTopologyDynamicCoresAnnotation]; isDynamicCores {
		// Swap cores and sockets.
		cores, sockets = sockets, cores
	}

	if cores > 0 && sockets > 0 {
		return v1alpha2.Topology{
			CoresPerSocket: cores,
			Sockets:        sockets,
		}
	}

	cores = h.getCoresByKVVMI(kvvmi)
	sockets, coresPerSocket, _ := vm.CalculateCoresAndSockets(cores)
	return v1alpha2.Topology{CoresPerSocket: coresPerSocket, Sockets: sockets}
}

func (h *StatisticHandler) syncPods(changed *v1alpha2.VirtualMachine, pod *corev1.Pod, pods *corev1.PodList) {
	if changed == nil {
		return
	}
	if pods == nil {
		changed.Status.VirtualMachinePods = nil
		return
	}
	virtualMachinePods := make([]v1alpha2.VirtualMachinePod, len(pods.Items))
	for i, p := range pods.Items {
		active := false
		if pod != nil && p.GetUID() == pod.GetUID() {
			active = true
		}
		virtualMachinePods[i] = v1alpha2.VirtualMachinePod{
			Name:   p.GetName(),
			Active: active,
		}
	}
	changed.Status.VirtualMachinePods = virtualMachinePods
}

func (h *StatisticHandler) syncStats(current, changed *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	if current == nil || changed == nil {
		return
	}
	phaseChanged := current.Status.Phase != changed.Status.Phase

	var stats v1alpha2.VirtualMachineStats

	if current.Status.Stats != nil {
		stats = *current.Status.Stats.DeepCopy()
	}
	pts := NewPhaseTransitions(stats.PhasesTransitions, current.Status.Phase, changed.Status.Phase)

	stats.PhasesTransitions = pts

	launchTimeDuration := stats.LaunchTimeDuration

	switch changed.Status.Phase {
	case v1alpha2.MachinePending, v1alpha2.MachineStopped:
		launchTimeDuration.WaitingForDependencies = nil
		launchTimeDuration.VirtualMachineStarting = nil
		launchTimeDuration.GuestOSAgentStarting = nil
	case v1alpha2.MachineStarting:
		launchTimeDuration.VirtualMachineStarting = nil
		launchTimeDuration.GuestOSAgentStarting = nil

		if phaseChanged {
			for i := len(pts) - 1; i > 0; i-- {
				pt := pts[i]
				ptPrev := pts[i-1]
				if pt.Phase == v1alpha2.MachineStarting && ptPrev.Phase == v1alpha2.MachinePending {
					launchTimeDuration.WaitingForDependencies = &metav1.Duration{Duration: pt.Timestamp.Sub(pts[i-1].Timestamp.Time)}
					break
				}
			}
		}
	case v1alpha2.MachineRunning:
		if kvvmi != nil && osInfoIsEmpty(kvvmi.Status.GuestOSInfo) {
			launchTimeDuration.GuestOSAgentStarting = nil
		}

		for i := len(pts) - 1; i > 0; i-- {
			pt := pts[i]
			ptPrev := pts[i-1]

			if pt.Phase == v1alpha2.MachineRunning {
				if phaseChanged && ptPrev.Phase == v1alpha2.MachineStarting {
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
	syncLastStartTime(changed, kvvmi)
}

func syncLastStartTime(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	running := getRunningCondition(vm)
	if running == nil || running.Status != metav1.ConditionTrue {
		if vm.Status.Stats != nil {
			vm.Status.Stats.LastStartTime = nil
		}
		return
	}

	kvvmiRunningAt := getKVVMIRunningPhaseTransitionTimestamp(kvvmi)
	if kvvmiRunningAt != nil && running.LastTransitionTime.Sub(kvvmiRunningAt.Time).Abs() > lastStartTimePhaseTransitionMaxDiff {
		running.LastTransitionTime = *kvvmiRunningAt.DeepCopy()
	}

	if vm.Status.Stats == nil {
		vm.Status.Stats = &v1alpha2.VirtualMachineStats{}
	}
	vm.Status.Stats.LastStartTime = running.LastTransitionTime.DeepCopy()
}

func getRunningCondition(vm *v1alpha2.VirtualMachine) *metav1.Condition {
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == vmcondition.TypeRunning.String() {
			return &vm.Status.Conditions[i]
		}
	}

	return nil
}

func getKVVMIRunningPhaseTransitionTimestamp(kvvmi *virtv1.VirtualMachineInstance) *metav1.Time {
	if kvvmi == nil {
		return nil
	}

	for i := len(kvvmi.Status.PhaseTransitionTimestamps) - 1; i >= 0; i-- {
		transition := kvvmi.Status.PhaseTransitionTimestamps[i]
		if transition.Phase == virtv1.Running {
			return &transition.PhaseTransitionTimestamp
		}
	}

	return nil
}

func osInfoIsEmpty(info virtv1.VirtualMachineInstanceGuestOSInfo) bool {
	var emptyOSInfo virtv1.VirtualMachineInstanceGuestOSInfo
	return emptyOSInfo == info
}

func NewPhaseTransitions(phaseTransitions []v1alpha2.VirtualMachinePhaseTransitionTimestamp, oldPhase, newPhase v1alpha2.MachinePhase) []v1alpha2.VirtualMachinePhaseTransitionTimestamp {
	now := metav1.NewTime(time.Now().Truncate(time.Second))

	if oldPhase != newPhase {
		phaseTransitions = append(phaseTransitions, v1alpha2.VirtualMachinePhaseTransitionTimestamp{
			Phase:     newPhase,
			Timestamp: now,
		})
	}
	if len(phaseTransitions) > 5 {
		return phaseTransitions[len(phaseTransitions)-5:]
	}
	return phaseTransitions
}
