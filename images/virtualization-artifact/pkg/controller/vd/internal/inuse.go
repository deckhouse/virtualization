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
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type reasonString struct {
	value string
}

func (rs reasonString) String() string {
	return rs.value
}

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	inUseCondition, ok := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if !ok {
		cb := conditions.NewConditionBuilder(vdcondition.InUseType).
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Generation(vd.Generation)
		conditions.SetCondition(cb, &vd.Status.Conditions)
		inUseCondition = cb.Condition()
	}

	usedByVM, usedByImage := false, false

	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		if h.isVDAttachedToVM(vd.GetName(), vm) {
			if vm.Status.Phase != virtv2.MachineStopped {
				usedByVM = isVMCanStart(vm.Status.Conditions)

				if usedByVM {
					break
				}
			} else {
				kvvm, err := object.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, h.client, &virtv1.VirtualMachine{})
				if err != nil {
					return reconcile.Result{}, fmt.Errorf("error getting kvvms: %w", err)
				}

				if kvvm != nil && kvvm.Status.StateChangeRequests != nil {
					usedByVM = true
					break
				}

				podList := corev1.PodList{}
				err = h.client.List(ctx, &podList, &client.ListOptions{
					Namespace:     vm.GetNamespace(),
					LabelSelector: labels.SelectorFromSet(map[string]string{virtv1.VirtualMachineNameLabel: vm.GetName()}),
				})
				if err != nil {
					return reconcile.Result{}, fmt.Errorf("unable to list virt-launcher Pod for VM %q: %w", vm.GetName(), err)
				}

				for _, pod := range podList.Items {
					if pod.Status.Phase == corev1.PodRunning {
						usedByVM = true
						break
					}
				}
			}
		}
	}

	var vis virtv2.VirtualImageList
	err = h.client.List(ctx, &vis, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting virtual images: %w", err)
	}

	allowedPhases := []virtv2.ImagePhase{virtv2.ImageProvisioning, virtv2.ImagePending}

	for _, vi := range vis.Items {
		if slices.Contains(allowedPhases, vi.Status.Phase) &&
			vi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef &&
			vi.Spec.DataSource.ObjectRef != nil &&
			vi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualDiskKind &&
			vi.Spec.DataSource.ObjectRef.Name == vd.Name {
			usedByImage = true
			break
		}
	}

	var cvis virtv2.ClusterVirtualImageList
	err = h.client.List(ctx, &cvis, &client.ListOptions{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting cluster virtual images: %w", err)
	}
	for _, cvi := range cvis.Items {
		if slices.Contains(allowedPhases, cvi.Status.Phase) &&
			cvi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef &&
			cvi.Spec.DataSource.ObjectRef != nil &&
			cvi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualDiskKind &&
			cvi.Spec.DataSource.ObjectRef.Name == vd.Name {
			usedByImage = true
		}
	}

	cb := conditions.NewConditionBuilder(vdcondition.InUseType)
	switch {
	case usedByVM && inUseCondition.Status != metav1.ConditionTrue:
		cb.
			Generation(vd.Generation).
			Status(metav1.ConditionTrue).
			Reason(vdcondition.AttachedToVirtualMachine).
			Message("")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	case usedByImage && inUseCondition.Status != metav1.ConditionTrue:
		cb.
			Generation(vd.Generation).
			Status(metav1.ConditionTrue).
			Reason(vdcondition.UsedForImageCreation).
			Message("")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	default:
		setNotInUse := false

		if inUseCondition.Reason == vdcondition.AttachedToVirtualMachine.String() && !usedByVM {
			setNotInUse = true
		}

		if inUseCondition.Reason == vdcondition.UsedForImageCreation.String() && !usedByImage {
			setNotInUse = true
		}

		if setNotInUse {
			cb.Generation(vd.Generation).Status(metav1.ConditionFalse).Reason(vdcondition.NotInUse).Message("")
			conditions.SetCondition(cb, &vd.Status.Conditions)
			// TODO redesign the handler's operation to change the logic for updating the Reason on the fly without intermediate switching to False and remove the requeue.
			return reconcile.Result{Requeue: true}, nil
		}
	}

	cb.Generation(vd.Generation).
		Status(inUseCondition.Status).
		Message(inUseCondition.Message).
		Reason(reasonString{inUseCondition.Reason})
	conditions.SetCondition(cb, &vd.Status.Conditions)
	return reconcile.Result{}, nil
}

func (h InUseHandler) isVDAttachedToVM(vdName string, vm virtv2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.DiskDevice && bda.Name == vdName {
			return true
		}
	}

	return false
}

func isVMCanStart(conditions []metav1.Condition) bool {
	critConditions := []string{
		vmcondition.TypeIPAddressReady.String(),
		vmcondition.TypeClassReady.String(),
	}

	for _, c := range conditions {
		if slices.Contains(critConditions, c.Type) && c.Status == metav1.ConditionFalse {
			return false
		}
	}

	return true
}
