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

package vmop

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func TestIsTerminalCompletedCondition(t *testing.T) {
	tests := []struct {
		name string
		cond metav1.Condition
		want bool
	}{
		{
			name: "migration completed reason is terminal",
			cond: metav1.Condition{Status: metav1.ConditionTrue, Reason: vmopcondition.ReasonMigrationCompleted.String()},
			want: true,
		},
		{
			name: "target disk error reason is terminal",
			cond: metav1.Condition{Status: metav1.ConditionFalse, Reason: vmopcondition.ReasonTargetDiskError.String()},
			want: true,
		},
		{
			name: "aborted reason is terminal",
			cond: metav1.Condition{Status: metav1.ConditionFalse, Reason: vmopcondition.ReasonAborted.String()},
			want: true,
		},
		{
			name: "not converging reason is terminal",
			cond: metav1.Condition{Status: metav1.ConditionFalse, Reason: vmopcondition.ReasonNotConverging.String()},
			want: true,
		},
		{
			name: "in progress reason is not terminal",
			cond: metav1.Condition{Status: metav1.ConditionFalse, Reason: vmopcondition.ReasonSyncing.String()},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTerminalCompletedCondition(tt.cond); got != tt.want {
				t.Fatalf("isTerminalCompletedCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewDataMetric_SetsFinishedAtForTerminalMigrationReasons(t *testing.T) {
	finishedAt := metav1.NewTime(time.Unix(1710000000, 0))
	vmop := &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "vmop-test",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0)),
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeMigrate,
			VirtualMachine: "test-vm",
		},
		Status: v1alpha2.VirtualMachineOperationStatus{
			Phase: v1alpha2.VMOPPhaseFailed,
			Conditions: []metav1.Condition{
				{
					Type:               vmopcondition.TypeCompleted.String(),
					Status:             metav1.ConditionFalse,
					Reason:             vmopcondition.ReasonTargetUnschedulable.String(),
					LastTransitionTime: finishedAt,
				},
			},
		},
	}

	metric := newDataMetric(vmop)
	if metric == nil {
		t.Fatal("expected metric to be created")
	}
	if metric.FinishedAt != finishedAt.Unix() {
		t.Fatalf("expected FinishedAt=%d, got %d", finishedAt.Unix(), metric.FinishedAt)
	}
}
