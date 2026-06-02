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

package validators

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type HotplugResourcesValidator struct {
	client client.Client
}

func NewHotplugResourcesValidator(client client.Client) *HotplugResourcesValidator {
	return &HotplugResourcesValidator{
		client: client,
	}
}

func (v *HotplugResourcesValidator) ValidateCreate(_ context.Context, _ *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, nil
}

func (v *HotplugResourcesValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if !isHotplugResourcesChanged(oldVM, newVM) {
		return nil, nil
	}

	if err := v.validateProjectQuota(ctx, newVM); err != nil {
		return nil, err
	}

	return nil, nil
}

func isHotplugResourcesChanged(oldVM, newVM *v1alpha2.VirtualMachine) bool {
	if oldVM.Spec.CPU.Cores != newVM.Spec.CPU.Cores {
		return true
	}
	if oldVM.Spec.CPU.CoreFraction != newVM.Spec.CPU.CoreFraction {
		return true
	}
	return oldVM.Spec.Memory.Size.Cmp(newVM.Spec.Memory.Size) != common.CmpEqual
}

func (v *HotplugResourcesValidator) validateProjectQuota(ctx context.Context, newVM *v1alpha2.VirtualMachine) error {
	newCPU, newMemory, err := getHotplugRequests(newVM)
	if err != nil {
		return err
	}

	var quotaList corev1.ResourceQuotaList
	if err = v.client.List(ctx, &quotaList, client.InNamespace(newVM.GetNamespace())); err != nil {
		return fmt.Errorf("list project quotas: %w", err)
	}

	for i := range quotaList.Items {
		quota := &quotaList.Items[i]
		if err = checkQuotaForResource(quota, corev1.ResourceRequestsCPU, newCPU); err != nil {
			return err
		}
		if err = checkQuotaForResource(quota, corev1.ResourceRequestsMemory, newMemory); err != nil {
			return err
		}
	}

	return nil
}

func getHotplugRequests(newVM *v1alpha2.VirtualMachine) (newCPU, newMemory resource.Quantity, err error) {
	var newCPUReq *resource.Quantity

	newCPUReq, err = kvbuilder.GetCPURequest(newVM.Spec.CPU.Cores, newVM.Spec.CPU.CoreFraction)
	if err != nil {
		return resource.Quantity{}, resource.Quantity{}, fmt.Errorf("calculate new CPU request: %w", err)
	}

	return *newCPUReq, newVM.Spec.Memory.Size, nil
}

func checkQuotaForResource(quota *corev1.ResourceQuota, resourceName corev1.ResourceName, newReq resource.Quantity) error {
	hard, hasHard := quota.Status.Hard[resourceName]
	if !hasHard {
		hard, hasHard = quota.Spec.Hard[resourceName]
	}
	if !hasHard {
		return nil
	}

	used := quota.Status.Used[resourceName]
	free := hard.DeepCopy()
	free.Sub(used)
	if free.Sign() < 0 {
		free.Set(0)
	}

	if newReq.Cmp(hard) == common.CmpGreater {
		return fmt.Errorf("%s request %s exceeds project quota %q hard limit %s", resourceName, newReq.String(), quota.GetName(), hard.String())
	}

	duringMigration := used.DeepCopy()
	duringMigration.Add(newReq)
	if duringMigration.Cmp(hard) == common.CmpGreater {
		return fmt.Errorf(
			"insufficient project quota %q for hotplug migration %s: required additional %s, available %s",
			quota.GetName(),
			resourceName,
			newReq.String(),
			free.String(),
		)
	}

	return nil
}
