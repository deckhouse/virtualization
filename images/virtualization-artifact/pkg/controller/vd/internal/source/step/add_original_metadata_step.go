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

package step

import (
	"context"
	"encoding/json"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

type AddOriginalMetadataStep struct {
	recorder eventrecord.EventRecorderLogger
	client   client.Client
	cb       *conditions.ConditionBuilder
}

func NewAddOriginalMetadataStep(
	recorder eventrecord.EventRecorderLogger,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *AddOriginalMetadataStep {
	return &AddOriginalMetadataStep{
		recorder: recorder,
		client:   client,
		cb:       cb,
	}
}

func (s AddOriginalMetadataStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	s.recorder.Event(
		vd,
		corev1.EventTypeNormal,
		v1alpha2.ReasonMetadataSyncStarted,
		"The original metadata sync has started",
	)

	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}, s.client, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch virtual disk snapshot: %w", err)
	}

	if vdSnapshot == nil {
		vd.Status.Phase = v1alpha2.DiskPending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.AddingOriginalMetadataNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q not found.", vd.Spec.DataSource.ObjectRef.Name))
		return &reconcile.Result{}, nil
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{Name: vdSnapshot.Status.VolumeSnapshotName, Namespace: vdSnapshot.Namespace}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("fetch volume snapshot: %w", err)
	}

	if vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady || vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		vd.Status.Phase = v1alpha2.DiskPending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.AddingOriginalMetadataNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q is not ready to use.", vdSnapshot.Name))
		return &reconcile.Result{}, nil
	}

	var (
		isMetadataAdded        bool
		originalAnnotationsMap map[string]string
		originalLabelsMap      map[string]string
	)

	if vs.Annotations != nil {
		if vs.Annotations[annotations.AnnVirtualDiskOriginalLabels] != "" {
			err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalLabels]), &originalLabelsMap)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal the original labels: %w", err)
			}
		}

		if vd.Labels == nil {
			vd.Labels = make(map[string]string)
		}
		for key, originalvalue := range originalLabelsMap {
			if currentValue, exists := vd.Labels[key]; !exists || currentValue != originalvalue {
				vd.Labels[key] = originalvalue
				isMetadataAdded = true
			}
		}

		if vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations] != "" {
			err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations]), &originalAnnotationsMap)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal the original annotations: %w", err)
			}
		}

		if vd.Annotations == nil {
			vd.Annotations = make(map[string]string)
		}
		for key, originalvalue := range originalAnnotationsMap {
			if currentValue, exists := vd.Annotations[key]; !exists || (key != lastAppliedConfigAnnotation && currentValue != originalvalue) {
				vd.Annotations[key] = originalvalue
				isMetadataAdded = true
			}
		}
	}

	if isMetadataAdded {
		return &reconcile.Result{}, nil
	}

	return nil, nil
}
