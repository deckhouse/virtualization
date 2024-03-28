package kvvm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
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
	return GetVMPod(kvvmi, &podList), nil
}

func GetVMPod(kvvmi *virtv1.VirtualMachineInstance, podList *corev1.PodList) *corev1.Pod {
	if len(podList.Items) == 0 {
		return nil
	}
	if len(podList.Items) == 1 {
		return &podList.Items[0]
	}

	// If migration is completed - return the target pod.
	if kvvmi != nil && kvvmi.Status.MigrationState != nil && kvvmi.Status.MigrationState.Completed {
		for _, pod := range podList.Items {
			if pod.Name == kvvmi.Status.MigrationState.TargetPod {
				return &pod
			}
		}
	}

	// Return the first Running Pod or just a first Pod.
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return &pod
		}
	}
	return &podList.Items[0]
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
	return helper.DeleteObject(ctx, cli, pod, opts)
}
