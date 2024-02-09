package kvvm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

// PatchRunStrategy returns JSON merge patch to set 'runStrategy' field to the desired value
// and reset deprecated 'running' field to null.
func PatchRunStrategy(runStrategy virtv1.VirtualMachineRunStrategy) client.Patch {
	runStrategyPatch := fmt.Sprintf(`{"spec":{"runStrategy": "%s", "running": null}}`, runStrategy)
	return client.RawPatch(types.MergePatchType, []byte(runStrategyPatch))
}

// GetRunStrategy returns runStrategy field value.
func GetRunStrategy(kvvm *virtv1.VirtualMachine) virtv1.VirtualMachineRunStrategy {
	if kvvm == nil || kvvm.Spec.RunStrategy == nil {
		return virtv1.RunStrategyUnknown
	}
	return *kvvm.Spec.RunStrategy
}

// FindPodByKVVMI returns pod by kvvmi.
func FindPodByKVVMI(ctx context.Context, cli client.Client, kvvmi *virtv1.VirtualMachineInstance) (*corev1.Pod, error) {
	if kvvmi == nil {
		return nil, fmt.Errorf("kvvmi must not be empty")
	}
	labelSelector, err := labels.Parse(fmt.Sprintf("%s=virt-launcher,%s=%s", virtv1.AppLabel, virtv1.CreatedByLabel, string(kvvmi.UID)))
	if err != nil {
		return nil, err
	}
	podList := corev1.PodList{}
	err = cli.List(ctx, &podList, &client.ListOptions{Namespace: kvvmi.GetNamespace(), LabelSelector: labelSelector})
	if err != nil || len(podList.Items) == 0 {
		return nil, err
	}
	if len(podList.Items) == 1 {
		return &podList.Items[0], nil
	}
	// Next, if pods are > 0
	// If migration is completed - return the target pod.
	if kvvmi.Status.MigrationState != nil && kvvmi.Status.MigrationState.Completed {
		for _, pod := range podList.Items {
			if pod.Name == kvvmi.Status.MigrationState.TargetPod {
				return &pod, nil
			}
		}
	}
	// return the first running pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return &pod, nil
		}
	}
	return &podList.Items[0], nil
}

// DeletePodByKVVMI deletes pod by kvvmi.
func DeletePodByKVVMI(ctx context.Context, cli client.Client, kvvmi *virtv1.VirtualMachineInstance, opts client.DeleteOption) error {
	pod, err := FindPodByKVVMI(ctx, cli, kvvmi)
	if err != nil {
		return err
	}
	if pod == nil {
		return nil
	}
	return cli.Delete(ctx, pod, opts)
}

// GetChangeRequest returns the stop/start patch.
func GetChangeRequest(vm *virtv1.VirtualMachine, changes ...virtv1.VirtualMachineStateChangeRequest) ([]byte, error) {
	jp := patch.NewJsonPatch()
	verb := patch.PatchAddOp
	// Special case: if there's no status field at all, add one.
	newStatus := virtv1.VirtualMachineStatus{}
	if equality.Semantic.DeepEqual(vm.Status, newStatus) {
		newStatus.StateChangeRequests = changes
		jp.Append(patch.NewJsonPatchOperation(verb, "/status", newStatus))
	} else {
		failOnConflict := true
		if len(changes) == 1 && changes[0].Action == virtv1.StopRequest {
			// If this is a stopRequest, replace all existing StateChangeRequests.
			failOnConflict = false
		}
		if len(vm.Status.StateChangeRequests) != 0 {
			if failOnConflict {
				return nil, fmt.Errorf("unable to complete request: stop/start already underway")
			} else {
				verb = patch.PatchReplaceOp
			}
		}
		jp.Append(patch.NewJsonPatchOperation(verb, "/status/stateChangeRequests", changes))
	}
	if vm.Status.StartFailure != nil {
		jp.Append(patch.NewJsonPatchOperation(patch.PatchRemoveOp, "/status/startFailure", nil))
	}
	return jp.Bytes()
}

// StartKVVM starts kvvm.
func StartKVVM(ctx context.Context, cli client.Client, kvvm *virtv1.VirtualMachine) error {
	if kvvm == nil {
		return fmt.Errorf("kvvm must not be empty")
	}
	jp, err := GetChangeRequest(kvvm,
		virtv1.VirtualMachineStateChangeRequest{Action: virtv1.StartRequest})
	if err != nil {
		return err
	}
	return cli.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
}

// StopKVVM stops kvvm.
func StopKVVM(ctx context.Context, cli client.Client, kvvmi *virtv1.VirtualMachineInstance, force bool) error {
	if kvvmi == nil {
		return fmt.Errorf("kvvmi must not be empty")
	}
	if err := cli.Delete(ctx, kvvmi, &client.DeleteOptions{}); err != nil {
		return err
	}
	if force {
		return DeletePodByKVVMI(ctx, cli, kvvmi, &client.DeleteOptions{GracePeriodSeconds: util.GetPointer(int64(0))})
	}
	return nil
}

// RestartKVVM restarts kvvm.
func RestartKVVM(ctx context.Context, cli client.Client, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, force bool) error {
	if kvvm == nil {
		return fmt.Errorf("kvvm must not be empty")
	}
	if kvvmi == nil {
		return fmt.Errorf("kvvmi must not be empty")
	}

	jp, err := GetChangeRequest(kvvm,
		virtv1.VirtualMachineStateChangeRequest{Action: virtv1.StopRequest, UID: &kvvmi.UID},
		virtv1.VirtualMachineStateChangeRequest{Action: virtv1.StartRequest})
	if err != nil {
		return err
	}

	err = cli.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
	if err != nil {
		return err
	}
	if force {
		return DeletePodByKVVMI(ctx, cli, kvvmi, &client.DeleteOptions{GracePeriodSeconds: util.GetPointer(int64(0))})
	}
	return nil
}
