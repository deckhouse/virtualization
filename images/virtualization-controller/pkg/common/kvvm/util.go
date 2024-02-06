package kvvm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func FindPodByKVVMI(ctx context.Context, cli client.Client, kvvmi *virtv1.VirtualMachineInstance) (*corev1.Pod, error) {
	if kvvmi == nil {
		return nil, fmt.Errorf("kvvmi must not be empty")
	}
	labelSelector, err := labels.Parse(fmt.Sprintf(virtv1.AppLabel + "=virt-launcher," + virtv1.CreatedByLabel + "=" + string(kvvmi.UID)))
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
	if kvvmi.Status.MigrationState != nil && kvvmi.Status.MigrationState.Completed {
		for _, pod := range podList.Items {
			if pod.Name == kvvmi.Status.MigrationState.TargetPod {
				return &pod, nil
			}
		}
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return &pod, nil
		}
	}
	return &podList.Items[0], nil
}

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
