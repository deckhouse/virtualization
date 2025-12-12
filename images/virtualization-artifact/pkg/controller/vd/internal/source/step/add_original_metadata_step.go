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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
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
	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}, s.client, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		err = fmt.Errorf("failed to fetch the virtual disk snapshot: %w", err)
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return nil, err
	}

	if vdSnapshot == nil {
		vd.Status.Phase = v1alpha2.DiskPending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q not found.", vd.Spec.DataSource.ObjectRef.Name))
		return &reconcile.Result{}, nil
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{Name: vdSnapshot.Status.VolumeSnapshotName, Namespace: vdSnapshot.Namespace}, s.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		err = fmt.Errorf("failed to fetch the volume snapshot: %w", err)
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return nil, err
	}

	if vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady || vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		vd.Status.Phase = v1alpha2.DiskPending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(fmt.Sprintf("VirtualDiskSnapshot %q is not ready to use.", vdSnapshot.Name))
		return &reconcile.Result{}, nil
	}

	areAnnotationsAdded, err := setOriginalAnnotations(vd, vs)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to set original annotations: %w", err)
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(wrappedErr.Error() + "."))
		return nil, wrappedErr
	}

	areLabelsAdded, err := setOriginalLabels(vd, vs)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to set original labels: %w", err)
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(wrappedErr.Error() + "."))
		return nil, wrappedErr
	}

	// Ensure that new metadata is applied correctly because a conflict error can occur
	// when updating the virtual disk resource. Therefore, this step should finish
	// with reconciliation if the metadata has changed.
	if areAnnotationsAdded || areLabelsAdded {
		msg := "The original metadata sync has started"
		s.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonMetadataSyncStarted,
			msg,
		)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message(msg)
		return &reconcile.Result{}, nil
	}

	return nil, nil
}

func setOriginalAnnotations(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) (bool, error) {
	var originalAnnotationsMap map[string]string
	if vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations] != "" {
		err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations]), &originalAnnotationsMap)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal the original annotations: %w", err)
		}
	}

	if vd.Annotations == nil {
		vd.Annotations = make(map[string]string)
	}

	var areAnnotationsAdded bool
	for key, originalvalue := range originalAnnotationsMap {
		if key == lastAppliedConfigAnnotation {
			continue
		}

		if _, exists := vd.Annotations[key]; !exists {
			vd.Annotations[key] = originalvalue
			areAnnotationsAdded = true
		}
	}

	return areAnnotationsAdded, nil
}

func setOriginalLabels(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) (bool, error) {
	var originalLabelsMap map[string]string
	if vs.Annotations[annotations.AnnVirtualDiskOriginalLabels] != "" {
		err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalLabels]), &originalLabelsMap)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal the original labels: %w", err)
		}
	}

	if vd.Labels == nil {
		vd.Labels = make(map[string]string)
	}

	var areLabelsAdded bool
	for key, originalvalue := range originalLabelsMap {
		if _, exists := vd.Labels[key]; !exists {
			vd.Labels[key] = originalvalue
			areLabelsAdded = true
		}
	}

	return areLabelsAdded, nil
}
