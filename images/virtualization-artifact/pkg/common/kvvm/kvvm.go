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

package kvvm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
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

	return object.DeleteObject(ctx, cli, pod, opts)
}

func AddRestartAnnotation(ctx context.Context, cl client.Client, kvvm *virtv1.VirtualMachine) error {
	return object.EnsureAnnotation(ctx, cl, kvvm, annotations.AnnVmRestartRequested, "true")
}

func AddStartAnnotation(ctx context.Context, cl client.Client, kvvm *virtv1.VirtualMachine) error {
	return object.EnsureAnnotation(ctx, cl, kvvm, annotations.AnnVmStartRequested, "true")
}

func RemoveStartAnnotation(ctx context.Context, cl client.Client, kvvm *virtv1.VirtualMachine) error {
	return object.RemoveAnnotation(ctx, cl, kvvm, annotations.AnnVmStartRequested)
}

func RemoveRestartAnnotation(ctx context.Context, cl client.Client, kvvm *virtv1.VirtualMachine) error {
	return object.RemoveAnnotation(ctx, cl, kvvm, annotations.AnnVmRestartRequested)
}
