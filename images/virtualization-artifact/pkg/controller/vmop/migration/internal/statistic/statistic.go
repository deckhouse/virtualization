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
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	vmmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualmachine"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// observeMigration must be called after a successful status write: the handler decides the same
// final phase again on a retry, and only the comparison of the two objects keeps that from
// counting twice.
func observeMigration(current, changed *v1alpha2.VirtualMachineOperation, mig *virtv1.VirtualMachineInstanceMigration, pod *corev1.Pod) {
	if current == nil || changed == nil || mig == nil {
		return
	}
	if !commonvmop.IsMigration(changed) || commonvmop.IsFinished(current) || !commonvmop.IsFinished(changed) {
		return
	}

	result, ok := migrationResult(changed.Status.Phase)
	if !ok {
		return
	}

	migrationType := migrationType(changed.Spec.Type)
	for stage, d := range stages(changed, mig, pod) {
		vmmetrics.ObserveMigration(migrationType, stage, result, d)
	}
}

func migrationResult(phase v1alpha2.VMOPPhase) (string, bool) {
	switch phase {
	case v1alpha2.VMOPPhaseCompleted:
		return vmmetrics.MigrationResultSucceeded, true
	case v1alpha2.VMOPPhaseFailed:
		return vmmetrics.MigrationResultFailed, true
	default:
		return "", false
	}
}

func migrationType(t v1alpha2.VMOPType) string {
	switch t {
	case v1alpha2.VMOPTypeEvict:
		return vmmetrics.MigrationTypeEvict
	default:
		return vmmetrics.MigrationTypeMigrate
	}
}

// stages skips an interval with an unknown end rather than reporting a made-up zero.
//
// The preparation is bounded by the target pod, not by the phases: KubeVirt stamps the phases in
// one go, and on the stand Scheduling, Scheduled, PreparingTarget and TargetReady all carried a
// timestamp thirteen seconds after the pod had been ready.
func stages(vmop *v1alpha2.VirtualMachineOperation, mig *virtv1.VirtualMachineInstanceMigration, pod *corev1.Pod) map[string]time.Duration {
	stages := make(map[string]time.Duration, 4)

	accepted := vmop.CreationTimestamp
	state := mig.Status.MigrationState

	targetReady := containersReadyAt(pod)
	if pod != nil && targetReady != nil {
		stages[vmmetrics.MigrationStagePreparing] = targetReady.Sub(pod.CreationTimestamp.Time)

		waiting := pod.CreationTimestamp.Sub(accepted.Time)
		if state != nil && state.StartTimestamp != nil {
			waiting += state.StartTimestamp.Sub(targetReady.Time)
		}
		stages[vmmetrics.MigrationStageWaiting] = waiting
	} else if scheduling := phaseTime(mig, virtv1.MigrationScheduling); scheduling != nil {
		// The pod is gone: the phase at least bounds the queue before the target was chosen.
		stages[vmmetrics.MigrationStageWaiting] = scheduling.Sub(accepted.Time)
	}

	if state != nil && state.StartTimestamp != nil && state.EndTimestamp != nil {
		stages[vmmetrics.MigrationStageTransfer] = state.EndTimestamp.Sub(state.StartTimestamp.Time)
	}
	if state != nil && state.EndTimestamp != nil {
		stages[vmmetrics.MigrationStageTotal] = state.EndTimestamp.Sub(accepted.Time)
	}

	return stages
}

func containersReadyAt(pod *corev1.Pod) *metav1.Time {
	if pod == nil {
		return nil
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.ContainersReady && c.Status == corev1.ConditionTrue {
			t := c.LastTransitionTime
			return &t
		}
	}
	return nil
}

// The first entry of a phase is the one that counts: KubeVirt appends one per transition.
func phaseTime(mig *virtv1.VirtualMachineInstanceMigration, phase virtv1.VirtualMachineInstanceMigrationPhase) *metav1.Time {
	for _, t := range mig.Status.PhaseTransitionTimestamps {
		if t.Phase == phase {
			ts := t.PhaseTransitionTimestamp
			return &ts
		}
	}
	return nil
}

func NewObserver(migration *service.MigrationService) *Observer {
	return &Observer{migration: migration}
}

type Observer struct {
	migration *service.MigrationService
}

func (o *Observer) Observe(ctx context.Context, c client.Client, current, changed *v1alpha2.VirtualMachineOperation) {
	if current == nil || changed == nil {
		return
	}
	if !commonvmop.IsMigration(changed) || commonvmop.IsFinished(current) || !commonvmop.IsFinished(changed) {
		return
	}

	mig, err := o.migration.GetMigration(ctx, changed)
	if err != nil || mig == nil {
		return
	}

	observeMigration(current, changed, mig, targetPod(ctx, c, mig))
}

func targetPod(ctx context.Context, c client.Client, mig *virtv1.VirtualMachineInstanceMigration) *corev1.Pod {
	pods := &corev1.PodList{}
	err := c.List(ctx, pods, client.InNamespace(mig.Namespace), client.MatchingLabels{
		virtv1.AppLabel:          "virt-launcher",
		virtv1.MigrationJobLabel: string(mig.UID),
	})
	if err != nil || len(pods.Items) == 0 {
		return nil
	}
	return &pods.Items[0]
}
