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
	"encoding/json"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/merger"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameSyncMetadataHandler = "SyncMetadataHandler"

func NewSyncMetadataHandler(client client.Client) *SyncMetadataHandler {
	return &SyncMetadataHandler{client: client}
}

type SyncMetadataHandler struct {
	client client.Client
}

func (h *SyncMetadataHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if isDeletion(s.VirtualMachine().Current()) {
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if kvvm == nil {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()

	// Propagate user specified labels and annotations from the d8 VM to kubevirt VM.
	kvvmNewMetadata := &metav1.ObjectMeta{}
	kvvmMetaUpdated, err := PropagateVMMetadata(current, kvvm, kvvm, kvvmNewMetadata)
	if err != nil {
		return reconcile.Result{}, err
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Propagate user specified labels and annotations from the d8 VM to the kubevirt VirtualMachineInstance.
	if kvvmi != nil {
		kvvmiNewMetadata := &metav1.ObjectMeta{}
		metaUpdated, err := PropagateVMMetadata(current, kvvm, kvvmi, kvvmiNewMetadata)
		if err != nil {
			return reconcile.Result{}, err
		}

		if metaUpdated {
			if err = h.patchLabelsAndAnnotations(ctx, kvvmi, kvvmiNewMetadata); err != nil && !k8serrors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("failed to patch metadata KubeVirt VMI %q: %w", kvvmi.GetName(), err)
			}
		}
	}

	pods, err := s.Pods(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Propagate user specified labels and annotations from the d8 VM to the kubevirt virtual machine Pods.
	if pods != nil {
		for _, pod := range pods.Items {
			// Update only Running pods.
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			podNewMetadata := &metav1.ObjectMeta{}
			metaUpdated, err := PropagateVMMetadata(current, kvvm, &pod, podNewMetadata)
			if err != nil {
				return reconcile.Result{}, err
			}

			if metaUpdated {
				if err = h.patchLabelsAndAnnotations(ctx, &pod, podNewMetadata); err != nil {
					return reconcile.Result{}, fmt.Errorf("failed to patch KubeVirt Pod %q: %w", pod.GetName(), err)
				}
			}
		}
	}

	labelsChanged, err := SetLastPropagatedLabels(kvvmNewMetadata, current)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to set last propagated labels: %w", err)
	}

	annosChanged, err := SetLastPropagatedAnnotations(kvvmNewMetadata, current)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to set last propagated annotations: %w", err)
	}

	if labelsChanged || annosChanged || kvvmMetaUpdated {
		if err = h.patchLabelsAndAnnotations(ctx, kvvm, kvvmNewMetadata); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to patch metadata KubeVirt VM %q: %w", kvvm.GetName(), err)
		}
	}

	return reconcile.Result{}, nil
}

func (h *SyncMetadataHandler) Name() string {
	return nameSyncMetadataHandler
}

func (h *SyncMetadataHandler) patchLabelsAndAnnotations(ctx context.Context, obj client.Object, metadata *metav1.ObjectMeta) error {
	jp := patch.NewJSONPatch()

	newLabels := metadata.GetLabels()
	newAnnotations := metadata.GetAnnotations()

	// For KubeVirt VirtualMachine, also patch spec.template.metadata
	// to ensure consistency with future VMI instances.
	// KubeVirt doesn't trigger VMI restart on template metadata changes.
	kvvm, objIsKVVM := obj.(*virtv1.VirtualMachine)

	if newLabels != nil {
		jp.Append(
			patch.WithTest("/metadata/labels", obj.GetLabels()),
			patch.WithReplace("/metadata/labels", newLabels),
		)
		if objIsKVVM {
			currSpecTemplateLabels := kvvm.Spec.Template.ObjectMeta.Labels
			syncedSpecTemplateLabels := h.updateKVVMSpecTemplateMetadataLabels(currSpecTemplateLabels, newLabels)
			jp.Append(
				patch.WithTest("/spec/template/metadata/labels", currSpecTemplateLabels),
				patch.WithReplace("/spec/template/metadata/labels", syncedSpecTemplateLabels),
			)
		}
	}

	if newAnnotations != nil {
		jp.Append(
			patch.WithTest("/metadata/annotations", obj.GetAnnotations()),
			patch.WithReplace("/metadata/annotations", newAnnotations),
		)
		if objIsKVVM {
			currSpecTemplateAnno := kvvm.Spec.Template.ObjectMeta.Annotations
			syncedSpecTemplateAnno := h.updateKVVMSpecTemplateMetadataAnnotations(currSpecTemplateAnno, newAnnotations)
			jp.Append(
				patch.WithTest("/spec/template/metadata/annotations", currSpecTemplateAnno),
				patch.WithReplace("/spec/template/metadata/annotations", syncedSpecTemplateAnno),
			)
		}
	}

	bytes, err := jp.Bytes()
	if err != nil {
		return err
	}

	return h.client.Patch(ctx, obj, client.RawPatch(types.JSONPatchType, bytes))
}

// updateKVVMSpecTemplateMetadataAnnotations ensures that the special network annotation is present if it exists.
// It also removes well-known annotations that are dangerous to propagate.
func (h *SyncMetadataHandler) updateKVVMSpecTemplateMetadataAnnotations(currAnno, newAnno map[string]string) map[string]string {
	res := make(map[string]string, len(newAnno))
	for k, v := range newAnno {
		if k == annotations.AnnVMLastAppliedSpec || k == annotations.AnnVMClassLastAppliedSpec {
			continue
		}

		res[k] = v
	}

	if v, ok := currAnno[annotations.AnnNetworksSpec]; ok {
		res[annotations.AnnNetworksSpec] = v
	}

	if v, ok := currAnno[virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation]; ok {
		res[virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation] = v
	}

	if v, ok := currAnno[netmanager.AnnoIPAddressCNIRequest]; ok {
		res[netmanager.AnnoIPAddressCNIRequest] = v
	}

	return commonvm.RemoveNonPropagatableAnnotations(res)
}

// updateKVVMSpecTemplateMetadataLabels ensures that the special labels is present if it exists.
func (h *SyncMetadataHandler) updateKVVMSpecTemplateMetadataLabels(currLabels, newLabels map[string]string) map[string]string {
	res := make(map[string]string, len(newLabels))
	maps.Copy(res, newLabels)

	if v, ok := currLabels[annotations.SkipPodSecurityStandardsCheckLabel]; ok {
		res[annotations.SkipPodSecurityStandardsCheckLabel] = v
	}

	return res
}

// PropagateVMMetadata merges labels and annotations from the input VM into destination object.
// Attach related labels and some dangerous annotations are not copied.
// Return true if destination object was changed.
func PropagateVMMetadata(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, origObj client.Object, metadata *metav1.ObjectMeta) (bool, error) {
	// No changes if origObj is nil.
	if origObj == nil {
		return false, nil
	}

	metadata.Labels = make(map[string]string, len(origObj.GetLabels()))
	metadata.Annotations = make(map[string]string, len(origObj.GetAnnotations()))

	maps.Copy(metadata.Labels, origObj.GetLabels())
	maps.Copy(metadata.Annotations, origObj.GetAnnotations())

	// 1. Propagate labels.
	lastPropagatedLabels, err := GetLastPropagatedLabels(kvvm)
	if err != nil {
		return false, err
	}

	// Add label to prevent node shutdown.
	propagateLabels := merger.MergeLabels(
		vm.GetLabels(),
		map[string]string{
			annotations.InhibitNodeShutdownLabel: "",
		},
	)

	if !vm.Status.Resources.CPU.RuntimeOverhead.IsZero() {
		propagateLabels[annotations.QuotaDiscountCPU] = vm.Status.Resources.CPU.RuntimeOverhead.String()
	}
	if !vm.Status.Resources.Memory.RuntimeOverhead.IsZero() {
		propagateLabels[annotations.QuotaDiscountMemory] = vm.Status.Resources.Memory.RuntimeOverhead.String()
	}

	newLabels, labelsChanged := merger.ApplyMapChanges(metadata.Labels, lastPropagatedLabels, propagateLabels)
	if labelsChanged {
		metadata.SetLabels(newLabels)
	}

	// 1. Propagate annotations.
	lastPropagatedAnno, err := GetLastPropagatedAnnotations(kvvm)
	if err != nil {
		return false, err
	}

	// Remove dangerous annotations.
	curAnno := commonvm.RemoveNonPropagatableAnnotations(vm.GetAnnotations())

	newAnno, annoChanged := merger.ApplyMapChanges(metadata.Annotations, lastPropagatedAnno, curAnno)
	if annoChanged {
		metadata.SetAnnotations(newAnno)
	}

	return labelsChanged || annoChanged, nil
}

func GetLastPropagatedLabels(kvvm *virtv1.VirtualMachine) (map[string]string, error) {
	var lastPropagatedLabels map[string]string

	if kvvm.Annotations[annotations.LastPropagatedVMLabelsAnnotation] != "" {
		err := json.Unmarshal([]byte(kvvm.Annotations[annotations.LastPropagatedVMLabelsAnnotation]), &lastPropagatedLabels)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedLabels, nil
}

func SetLastPropagatedLabels(metadata *metav1.ObjectMeta, vm *v1alpha2.VirtualMachine) (bool, error) {
	data, err := json.Marshal(vm.GetLabels())
	if err != nil {
		return false, err
	}

	newAnnoValue := string(data)

	if metadata.Annotations[annotations.LastPropagatedVMLabelsAnnotation] == newAnnoValue {
		return false, nil
	}

	annotations.AddAnnotation(metadata, annotations.LastPropagatedVMLabelsAnnotation, newAnnoValue)
	return true, nil
}

func GetLastPropagatedAnnotations(kvvm *virtv1.VirtualMachine) (map[string]string, error) {
	var lastPropagatedAnno map[string]string

	if kvvm.Annotations[annotations.LastPropagatedVMAnnotationsAnnotation] != "" {
		err := json.Unmarshal([]byte(kvvm.Annotations[annotations.LastPropagatedVMAnnotationsAnnotation]), &lastPropagatedAnno)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedAnno, nil
}

func SetLastPropagatedAnnotations(metadata *metav1.ObjectMeta, vm *v1alpha2.VirtualMachine) (bool, error) {
	data, err := json.Marshal(commonvm.RemoveNonPropagatableAnnotations(vm.GetAnnotations()))
	if err != nil {
		return false, err
	}

	newAnnoValue := string(data)

	if metadata.Annotations[annotations.LastPropagatedVMAnnotationsAnnotation] == newAnnoValue {
		return false, nil
	}

	annotations.AddAnnotation(metadata, annotations.LastPropagatedVMAnnotationsAnnotation, newAnnoValue)
	return true, nil
}
