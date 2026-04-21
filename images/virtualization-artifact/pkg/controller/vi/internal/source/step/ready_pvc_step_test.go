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

package step

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type noopReadyPersistentVolumeClaimStepBounder struct{}

func (noopReadyPersistentVolumeClaimStepBounder) CleanUpSupplements(context.Context, supplements.Generator) (bool, error) {
	return true, nil
}

func noopEvent(client.Object, string, string, string) {}

func TestReadyPersistentVolumeClaimStepTakeUsesPVCCapacityForStoredAndUnpacked(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "image-pvc"},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("12Gi"),
			},
		},
	}

	vi := &v1alpha2.VirtualImage{}
	var recorder *eventrecord.EventRecorderLoggerMock
	recorder = &eventrecord.EventRecorderLoggerMock{
		EventFunc: noopEvent,
		WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
			return recorder
		},
	}
	cb := conditions.NewConditionBuilder(vicondition.ReadyType)
	step := NewReadyPersistentVolumeClaimStep(pvc, noopReadyPersistentVolumeClaimStepBounder{}, recorder, cb)

	_, err := step.Take(context.Background(), vi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vi.Status.Size.Stored != "12Gi" {
		t.Fatalf("expected stored size 12Gi, got %s", vi.Status.Size.Stored)
	}

	if vi.Status.Size.Unpacked != "12Gi" {
		t.Fatalf("expected unpacked size 12Gi, got %s", vi.Status.Size.Unpacked)
	}
}
