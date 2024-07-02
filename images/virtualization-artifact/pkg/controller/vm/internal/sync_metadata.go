package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	merger "github.com/deckhouse/virtualization-controller/pkg/common"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	metaUpdated, err := PropagateVMMetadata(current, kvvm, kvvm)
	if err != nil {
		return reconcile.Result{}, err
	}

	if metaUpdated {
		if err = h.client.Update(ctx, kvvm); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update metadata KubeVirt VM %q: %w", kvvm.GetName(), err)
		}
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Propagate user specified labels and annotations from the d8 VM to the kubevirt VirtualMachineInstance.
	if kvvmi != nil {
		metaUpdated, err = PropagateVMMetadata(current, kvvm, kvvmi)
		if err != nil {
			return reconcile.Result{}, err
		}

		if metaUpdated {
			if err = h.client.Update(ctx, kvvmi); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to update metadata KubeVirt VMI %q: %w", kvvmi.GetName(), err)
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
			metaUpdated, err = PropagateVMMetadata(current, kvvm, &pod)
			if err != nil {
				return reconcile.Result{}, err
			}

			if metaUpdated {
				if err = h.client.Update(ctx, &pod); err != nil {
					return reconcile.Result{}, fmt.Errorf("fauled to update KubeVirt Pod %q: %w", pod.GetName(), err)
				}
			}
		}
	}
	err = SetLastPropagatedLabels(kvvm, current)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to set last propagated labels: %w", err)
	}

	err = SetLastPropagatedAnnotations(kvvm, current)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to set last propagated annotations: %w", err)
	}

	return reconcile.Result{}, nil
}

func (h *SyncMetadataHandler) Name() string {
	return nameSyncMetadataHandler
}

// PropagateVMMetadata merges labels and annotations from the input VM into destination object.
// Attach related labels and some dangerous annotations are not copied.
// Return true if destination object was changed.
func PropagateVMMetadata(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, destObj client.Object) (bool, error) {
	// No changes if dest is nil.
	if destObj == nil {
		return false, nil
	}

	// 1. Propagate labels.
	lastPropagatedLabels, err := GetLastPropagatedLabels(kvvm)
	if err != nil {
		return false, err
	}

	newLabels, labelsChanged := merger.ApplyMapChanges(destObj.GetLabels(), lastPropagatedLabels, vm.GetLabels())
	if labelsChanged {
		destObj.SetLabels(newLabels)
	}

	// 1. Propagate annotations.
	lastPropagatedAnno, err := GetLastPropagatedAnnotations(kvvm)
	if err != nil {
		return false, err
	}

	// Remove dangerous annotations.
	curAnno := RemoveNonPropagatableAnnotations(vm.GetAnnotations())

	newAnno, annoChanged := merger.ApplyMapChanges(destObj.GetAnnotations(), lastPropagatedAnno, curAnno)
	if annoChanged {
		destObj.SetAnnotations(newAnno)
	}

	return labelsChanged || annoChanged, nil
}

func GetLastPropagatedLabels(kvvm *virtv1.VirtualMachine) (map[string]string, error) {
	var lastPropagatedLabels map[string]string

	if kvvm.Annotations[common.LastPropagatedVMLabelsAnnotation] != "" {
		err := json.Unmarshal([]byte(kvvm.Annotations[common.LastPropagatedVMLabelsAnnotation]), &lastPropagatedLabels)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedLabels, nil
}

func SetLastPropagatedLabels(kvvm *virtv1.VirtualMachine, vm *virtv2.VirtualMachine) error {
	data, err := json.Marshal(vm.GetLabels())
	if err != nil {
		return err
	}

	common.AddLabel(kvvm, common.LastPropagatedVMLabelsAnnotation, string(data))

	return nil
}

func GetLastPropagatedAnnotations(kvvm *virtv1.VirtualMachine) (map[string]string, error) {
	var lastPropagatedAnno map[string]string

	if kvvm.Annotations[common.LastPropagatedVMAnnotationsAnnotation] != "" {
		err := json.Unmarshal([]byte(kvvm.Annotations[common.LastPropagatedVMAnnotationsAnnotation]), &lastPropagatedAnno)
		if err != nil {
			return nil, err
		}
	}

	return lastPropagatedAnno, nil
}

func SetLastPropagatedAnnotations(kvvm *virtv1.VirtualMachine, vm *virtv2.VirtualMachine) error {
	data, err := json.Marshal(RemoveNonPropagatableAnnotations(vm.GetAnnotations()))
	if err != nil {
		return err
	}

	common.AddLabel(kvvm, common.LastPropagatedVMAnnotationsAnnotation, string(data))

	return nil
}

// RemoveNonPropagatableAnnotations removes well known annotations that are dangerous to propagate.
func RemoveNonPropagatableAnnotations(anno map[string]string) map[string]string {
	res := make(map[string]string)

	for k, v := range anno {
		if k == common.LastPropagatedVMAnnotationsAnnotation || k == common.LastPropagatedVMLabelsAnnotation {
			continue
		}

		if strings.HasPrefix(k, "kubectl.kubernetes.io") {
			continue
		}

		res[k] = v
	}
	return res
}
