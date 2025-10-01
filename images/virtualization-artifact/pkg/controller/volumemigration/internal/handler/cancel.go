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

package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const CancelHandlerName = "CancelHandler"

type CancelHandler struct {
	client client.Client
}

func NewCancelHandler(client client.Client) *CancelHandler {
	return &CancelHandler{
		client: client,
	}
}

func (h *CancelHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if commonvd.StorageClassChanged(vd) {
		return reconcile.Result{}, nil
	}

	migrating, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
	if migrating.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	vmName := commonvd.GetCurrentlyMountedVMName(vd)
	vmop, err := h.getActiveVolumeMigration(ctx, types.NamespacedName{Name: vmName, Namespace: vd.Namespace})
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmop != nil {
		return reconcile.Result{}, h.client.Delete(ctx, vmop)
	}

	return reconcile.Result{}, nil
}

func (h *CancelHandler) Name() string {
	return CancelHandlerName
}

func (h *CancelHandler) getActiveVolumeMigration(ctx context.Context, vmKey types.NamespacedName) (*v1alpha2.VirtualMachineOperation, error) {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, vmops, client.InNamespace(vmKey.Namespace))
	if err != nil {
		return nil, err
	}

	for _, vmop := range vmops.Items {
		if commonvmop.IsMigration(&vmop) &&
			commonvmop.IsInProgressOrPending(&vmop) &&
			vmop.Spec.VirtualMachine == vmKey.Name &&
			vmop.GetAnnotations()[annotations.AnnVMOPVolumeMigration] == "true" {
			return &vmop, nil
		}
	}

	return nil, nil
}
