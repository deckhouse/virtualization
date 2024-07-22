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
	"sort"
	"time"

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
	return reconcile.Result{}, nil
}

func (h *StatisticHandler) Name() string {
	return nameStatisticHandler
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

	for i, pt := range pts.Items {
		switch pt.Phase {
		case virtv2.MachineStarting:
			if i > 0 && pts.Items[i-1].Phase == phasePreviousPhase[pt.Phase] && current.Status.Phase != changed.Status.Phase {
				launchTimeDuration.WaitingForDependencies = &metav1.Duration{Duration: pt.Timestamp.Sub(pts.Items[i-1].Timestamp.Time)}
			}
		case virtv2.MachineRunning:
			if i > 0 && pts.Items[i-1].Phase == phasePreviousPhase[pt.Phase] && current.Status.Phase != changed.Status.Phase {
				launchTimeDuration.VirtualMachineStarting = &metav1.Duration{Duration: pt.Timestamp.Sub(pts.Items[i-1].Timestamp.Time)}
			}
			var empty virtv1.VirtualMachineInstanceGuestOSInfo
			if kvvmi != nil && empty == current.Status.GuestOSInfo && empty != kvvmi.Status.GuestOSInfo && !pt.Timestamp.IsZero() {
				launchTimeDuration.GuestOSAgentStarting = &metav1.Duration{Duration: time.Now().Truncate(time.Second).Sub(pt.Timestamp.Time)}
			}
		}
	}

	stats.LaunchTimeDuration = launchTimeDuration
	changed.Status.Stats = &stats
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
			if _, found = phasesTransitionsMap[previousPhase]; !found {
				phasesTransitionsMap[previousPhase] = virtv2.VirtualMachinePhaseTransitionTimestamp{
					Phase:     previousPhase,
					Timestamp: metav1.NewTime(t),
				}
				t = t.Add(-1 * time.Second)
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
