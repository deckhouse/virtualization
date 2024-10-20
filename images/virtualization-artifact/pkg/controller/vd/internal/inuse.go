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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	inUseCondition, ok := service.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if !ok {
		inUseCondition = metav1.Condition{
			Type:   vdcondition.InUseType,
			Status: metav1.ConditionUnknown,
		}

		service.SetCondition(inUseCondition, &vd.Status.Conditions)
	}

	var inUseForCreateImage, inUseInRunningVirtualMachine bool

	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		for _, bda := range vm.Status.BlockDeviceRefs {
			if bda.Kind == virtv2.DiskDevice && bda.Name == vd.GetName() {
				runningCondition, _ := service.GetCondition(vmcondition.TypeRunning.String(), vm.Status.Conditions)
				if runningCondition.Status == metav1.ConditionTrue || vm.Status.Phase == virtv2.MachineStarting {
					inUseInRunningVirtualMachine = true
				} else {
					kvvm, err := helper.FetchObject(ctx, types.NamespacedName{Namespace: vm.GetNamespace(), Name: vm.GetName()}, h.client, &virtv1.VirtualMachine{})
					if err != nil {
						return reconcile.Result{}, fmt.Errorf("error getting kubevirt virtual machine: %w", err)
					}

					if kvvm.Status.StateChangeRequests != nil {
						inUseInRunningVirtualMachine = true
					}
				}
			}
		}
	}

	var viList virtv2.VirtualImageList
	err = h.client.List(ctx, &viList, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting virtual images: %w", err)
	}

	for _, vi := range viList.Items {
		if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || vi.Spec.DataSource.ObjectRef.Name != vd.GetName() {
			continue
		}

		readyCondition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
		if readyCondition.Status != metav1.ConditionTrue {
			inUseForCreateImage = true
		}
	}

	var cviList virtv2.ClusterVirtualImageList
	err = h.client.List(ctx, &cviList, &client.ListOptions{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting cluster virtual images: %w", err)
	}

	for _, cvi := range cviList.Items {
		if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || cvi.Spec.DataSource.ObjectRef.Name != vd.GetName() && cvi.Spec.DataSource.ObjectRef.Namespace != vd.GetNamespace() {
			continue
		}

		readyCondition, _ := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
		if readyCondition.Status != metav1.ConditionTrue {
			inUseForCreateImage = true
		}
	}

	if inUseCondition.Status == metav1.ConditionFalse && inUseInRunningVirtualMachine {
		inUseCondition.Status = metav1.ConditionTrue
		inUseCondition.Reason = vdcondition.InUseInRunningVirtualMachine
	} else if inUseCondition.Reason == vdcondition.InUseInRunningVirtualMachine {
		inUseCondition.Status = metav1.ConditionFalse
		inUseCondition.Reason = vdcondition.NotInUse
	}

	if inUseCondition.Status == metav1.ConditionFalse && inUseForCreateImage {
		inUseCondition.Status = metav1.ConditionTrue
		inUseCondition.Reason = vdcondition.InUseForCreateImage
	} else if inUseCondition.Reason == vdcondition.InUseForCreateImage {
		inUseCondition.Status = metav1.ConditionFalse
		inUseCondition.Reason = vdcondition.NotInUse
	}

	inUseCondition.Message = ""
	service.SetCondition(inUseCondition, &vd.Status.Conditions)

	return reconcile.Result{}, nil
}
