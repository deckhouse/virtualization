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
	"time"

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

var imagePhasesUsingDisk = []virtv2.ImagePhase{virtv2.ImageProvisioning, virtv2.ImagePending}

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
	var err error

	if inUseCondition.Reason != vdcondition.UsedForImageCreation.String() {
		usedByVM, err = h.checkUsageByVM(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}

		if !usedByVM {
			usedByImage, err = h.checkImageUsage(ctx, vd)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		usedByImage, err = h.checkImageUsage(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}

		if !usedByImage {
			usedByVM, err = h.checkUsageByVM(ctx, vd)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	cb := conditions.NewConditionBuilder(vdcondition.InUseType)
	switch {
	case usedByVM:
		cb.Generation(vd.Generation).
			Status(metav1.ConditionTrue).
			Reason(vdcondition.AttachedToVirtualMachine).
			Message("").
			LastTransitionTime(time.Now())
	case usedByImage:
		cb.Generation(vd.Generation).
			Status(metav1.ConditionTrue).
			Reason(vdcondition.UsedForImageCreation).
			Message("").
			LastTransitionTime(time.Now())
	default:
		cb.Generation(vd.Generation).
			Status(metav1.ConditionFalse).
			Reason(vdcondition.NotInUse).
			Message("")
	}

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

func canStartVM(conditions []metav1.Condition) bool {
	critConditions := []string{
		vmcondition.TypeIPAddressReady.String(),
		vmcondition.TypeClassReady.String(),
		vmcondition.TypeProvisioningReady.String(),
	}

	for _, c := range conditions {
		if slices.Contains(critConditions, c.Type) && c.Status == metav1.ConditionFalse {
			return false
		}
	}

	return true
}

func (h InUseHandler) checkImageUsage(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	usedByImage, err := h.checkUsageByVI(ctx, vd)
	if err != nil {
		return false, err
	}
	if !usedByImage {
		usedByImage, err = h.checkUsageByCVI(ctx, vd)
		if err != nil {
			return false, err
		}
	}

	return usedByImage, nil
}

func (h InUseHandler) checkUsageByVM(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return false, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		if !h.isVDAttachedToVM(vd.GetName(), vm) {
			continue
		}

		switch vm.Status.Phase {
		case "":
			return false, nil

		case virtv2.MachinePending:
			usedByVM := canStartVM(vm.Status.Conditions)

			if usedByVM {
				return true, nil
			}
		case virtv2.MachineStopped:
			kvvm, err := object.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, h.client, &virtv1.VirtualMachine{})
			if err != nil {
				return false, fmt.Errorf("error getting kvvms: %w", err)
			}

			if kvvm != nil && kvvm.Status.StateChangeRequests != nil {
				return true, nil
			}

			podList := corev1.PodList{}
			err = h.client.List(ctx, &podList, &client.ListOptions{
				Namespace:     vm.GetNamespace(),
				LabelSelector: labels.SelectorFromSet(map[string]string{virtv1.VirtualMachineNameLabel: vm.GetName()}),
			})
			if err != nil {
				return false, fmt.Errorf("unable to list virt-launcher Pod for VM %q: %w", vm.GetName(), err)
			}

			for _, pod := range podList.Items {
				if pod.Status.Phase == corev1.PodRunning {
					return true, nil
				}
			}
		default:
			return true, nil
		}
	}

	return false, nil
}

func (h InUseHandler) checkUsageByVI(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	var vis virtv2.VirtualImageList
	err := h.client.List(ctx, &vis, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return false, fmt.Errorf("error getting virtual images: %w", err)
	}

	for _, vi := range vis.Items {
		if slices.Contains(imagePhasesUsingDisk, vi.Status.Phase) &&
			vi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef &&
			vi.Spec.DataSource.ObjectRef != nil &&
			vi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualDiskKind &&
			vi.Spec.DataSource.ObjectRef.Name == vd.Name {
			return true, nil
		}
	}

	return false, nil
}

func (h InUseHandler) checkUsageByCVI(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	var cvis virtv2.ClusterVirtualImageList
	err := h.client.List(ctx, &cvis, &client.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting cluster virtual images: %w", err)
	}
	for _, cvi := range cvis.Items {
		if slices.Contains(imagePhasesUsingDisk, cvi.Status.Phase) &&
			cvi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef &&
			cvi.Spec.DataSource.ObjectRef != nil &&
			cvi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualDiskKind &&
			cvi.Spec.DataSource.ObjectRef.Name == vd.Name {
			return true, nil
		}
	}

	return false, nil
}
