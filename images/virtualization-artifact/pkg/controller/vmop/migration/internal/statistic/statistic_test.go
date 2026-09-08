/*
Copyright 2026 Flant JSC

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

package statistic

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	vmmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualmachine"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func at(base time.Time, offset time.Duration) metav1.Time {
	return metav1.NewTime(base.Add(offset))
}

// Every phase carries the same timestamp, stamped long after the target pod was ready.
func stand(base time.Time) *virtv1.VirtualMachineInstanceMigration {
	start := at(base, 13*time.Second)
	end := at(base, 17*time.Second)
	return &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			Phase: virtv1.MigrationSucceeded,
			PhaseTransitionTimestamps: []virtv1.VirtualMachineInstanceMigrationPhaseTransitionTimestamp{
				{Phase: virtv1.MigrationPending, PhaseTransitionTimestamp: at(base, 0)},
				{Phase: virtv1.MigrationScheduling, PhaseTransitionTimestamp: at(base, 7*time.Second)},
				{Phase: virtv1.MigrationScheduled, PhaseTransitionTimestamp: at(base, 13*time.Second)},
				{Phase: virtv1.MigrationPreparingTarget, PhaseTransitionTimestamp: at(base, 13*time.Second)},
				{Phase: virtv1.MigrationTargetReady, PhaseTransitionTimestamp: at(base, 13*time.Second)},
				{Phase: virtv1.MigrationRunning, PhaseTransitionTimestamp: at(base, 13*time.Second)},
				{Phase: virtv1.MigrationSucceeded, PhaseTransitionTimestamp: at(base, 17*time.Second)},
			},
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp: &start,
				EndTimestamp:   &end,
			},
		},
	}
}

func targetPodAt(base time.Time, created, ready time.Duration) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: at(base, created)},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue, LastTransitionTime: at(base, ready)},
			},
		},
	}
}

func vmopAt(base time.Time, phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{Name: "vm-1-evict", CreationTimestamp: at(base, 0)},
		Spec:       v1alpha2.VirtualMachineOperationSpec{Type: v1alpha2.VMOPTypeEvict},
		Status:     v1alpha2.VirtualMachineOperationStatus{Phase: phase},
	}
}

func TestStages(t *testing.T) {
	base := time.Date(2026, 9, 5, 19, 32, 39, 0, time.UTC)
	got := stages(vmopAt(base, v1alpha2.VMOPPhaseCompleted), stand(base), targetPodAt(base, 0, time.Second))

	want := map[string]time.Duration{
		vmmetrics.MigrationStagePreparing: time.Second,
		vmmetrics.MigrationStageWaiting:   12 * time.Second,
		vmmetrics.MigrationStageTransfer:  4 * time.Second,
		vmmetrics.MigrationStageTotal:     17 * time.Second,
	}
	for stage, d := range want {
		if got[stage] != d {
			t.Errorf("%s: expected %s, got %s", stage, d, got[stage])
		}
	}

	sum := got[vmmetrics.MigrationStagePreparing] + got[vmmetrics.MigrationStageWaiting] +
		got[vmmetrics.MigrationStageTransfer]
	if sum != got[vmmetrics.MigrationStageTotal] {
		t.Errorf("the stages must add up to the total: %s vs %s", sum, got[vmmetrics.MigrationStageTotal])
	}
}

func TestStagesWaitingIsBothQueues(t *testing.T) {
	base := time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)
	mig := stand(base)
	start := at(base, 54*time.Second)
	end := at(base, 60*time.Second)
	mig.Status.MigrationState.StartTimestamp = &start
	mig.Status.MigrationState.EndTimestamp = &end

	got := stages(vmopAt(base, v1alpha2.VMOPPhaseCompleted), mig,
		targetPodAt(base, 10*time.Second, 14*time.Second))

	if got[vmmetrics.MigrationStagePreparing] != 4*time.Second {
		t.Errorf("preparing: expected 4s of getting the target ready, got %s",
			got[vmmetrics.MigrationStagePreparing])
	}
	if got[vmmetrics.MigrationStageWaiting] != 50*time.Second {
		t.Errorf("waiting: expected 10s before the pod plus 40s for a slot, got %s",
			got[vmmetrics.MigrationStageWaiting])
	}
}

func TestStagesWithoutATargetPodFallsBackToThePhase(t *testing.T) {
	base := time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)
	got := stages(vmopAt(base, v1alpha2.VMOPPhaseCompleted), stand(base), nil)

	if _, found := got[vmmetrics.MigrationStagePreparing]; found {
		t.Errorf("preparing must be absent without a target pod, got %s",
			got[vmmetrics.MigrationStagePreparing])
	}
	if got[vmmetrics.MigrationStageWaiting] != 7*time.Second {
		t.Errorf("waiting falls back to the queue before Scheduling, expected 7s, got %s",
			got[vmmetrics.MigrationStageWaiting])
	}
}

func TestStagesSkipsWhatNeverHappened(t *testing.T) {
	base := time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)
	mig := &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			Phase: virtv1.MigrationFailed,
			PhaseTransitionTimestamps: []virtv1.VirtualMachineInstanceMigrationPhaseTransitionTimestamp{
				{Phase: virtv1.MigrationPending, PhaseTransitionTimestamp: at(base, 0)},
				{Phase: virtv1.MigrationFailed, PhaseTransitionTimestamp: at(base, 30*time.Second)},
			},
		},
	}
	if got := stages(vmopAt(base, v1alpha2.VMOPPhaseFailed), mig, nil); len(got) != 0 {
		t.Errorf("a migration that never reached a stage has nothing to report, got %v", got)
	}
}

func TestObserveMigrationCountsOnlyTheTransitionToFinal(t *testing.T) {
	base := time.Date(2026, 9, 5, 18, 7, 5, 0, time.UTC)
	mig := stand(base)
	vmmetrics.MigrationDuration.Reset()

	inProgress := vmopAt(base, v1alpha2.VMOPPhaseInProgress)
	completed := vmopAt(base, v1alpha2.VMOPPhaseCompleted)

	observeMigration(inProgress, inProgress, mig, nil)
	if got := transfers(t); got != 0 {
		t.Fatalf("a migration in progress must not be counted, got %d", got)
	}

	observeMigration(inProgress, completed, mig, nil)
	if got := transfers(t); got != 1 {
		t.Fatalf("expected the finished migration to be counted once, got %d", got)
	}

	observeMigration(completed, completed, mig, nil)
	if got := transfers(t); got != 1 {
		t.Fatalf("expected no second observation, got %d", got)
	}
}

func transfers(t *testing.T) uint64 {
	t.Helper()
	return count(t, vmmetrics.MigrationTypeEvict, vmmetrics.MigrationStageTransfer, vmmetrics.MigrationResultSucceeded)
}

func count(t *testing.T, migrationType, stage, result string) uint64 {
	t.Helper()
	h, err := vmmetrics.MigrationDuration.GetMetricWithLabelValues(migrationType, stage, result)
	if err != nil {
		t.Fatal(err)
	}
	m := &dto.Metric{}
	if err := h.(prometheus.Metric).Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}
