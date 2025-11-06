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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var imagePhasesUsingDisk = []v1alpha2.ImagePhase{v1alpha2.ImageProvisioning, v1alpha2.ImagePending}

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	err := h.updateAttachedVirtualMachines(ctx, vd)
	if err != nil {
		return reconcile.Result{}, err
	}

	var (
		usedByVM         bool
		usedByImage      bool
		usedByDataExport bool
	)

	usedByVM = h.checkUsageByVM(vd)
	if !usedByVM {
		usedByImage, err = h.checkImageUsage(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	if !usedByVM && !usedByImage {
		usedByDataExport, err = h.checkDataExportUsage(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	cb := conditions.NewConditionBuilder(vdcondition.InUseType).Generation(vd.Generation)
	switch {
	case usedByVM:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.AttachedToVirtualMachine).
			Message("")
	case usedByImage:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.UsedForImageCreation).
			Message("")
	case usedByDataExport:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.UsedForDataExport).
			Message("")
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.NotInUse).
			Message("")
	}

	conditions.SetCondition(cb, &vd.Status.Conditions)
	return reconcile.Result{}, nil
}

func (h InUseHandler) isVDAttachedToVM(vdName string, vm v1alpha2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == v1alpha2.DiskDevice && bda.Name == vdName {
			return true
		}
	}

	return false
}

func (h InUseHandler) checkDataExportUsage(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	pvcName := vd.Status.Target.PersistentVolumeClaim
	if pvcName == "" {
		return false, nil
	}

	pvc, err := object.FetchObject(ctx, types.NamespacedName{Name: pvcName, Namespace: vd.Namespace}, h.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, fmt.Errorf("fetch pvc: %w", err)
	}
	if pvc == nil {
		return false, nil
	}

	return pvc.GetAnnotations()[annotations.AnnDataExportRequest] == "true", nil
}

func (h InUseHandler) checkImageUsage(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	// If disk is not ready, it cannot be used for create image
	if vd.Status.Phase != v1alpha2.DiskReady {
		return false, nil
	}

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

func (h InUseHandler) updateAttachedVirtualMachines(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	var vms v1alpha2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace:     vd.Namespace,
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMByVD, vd.Name),
	})
	if err != nil {
		return fmt.Errorf("error getting virtual machines: %w", err)
	}

	usageMap, err := h.getVirtualMachineUsageMap(ctx, vd, vms)
	if err != nil {
		return err
	}

	h.updateAttachedVirtualMachinesStatus(vd, usageMap)
	return nil
}

func (h InUseHandler) getVirtualMachineUsageMap(ctx context.Context, vd *v1alpha2.VirtualDisk, vms v1alpha2.VirtualMachineList) (map[string]bool, error) {
	usageMap := make(map[string]bool)

	for _, vm := range vms.Items {
		if !h.isVDAttachedToVM(vd.GetName(), vm) {
			continue
		}

		switch vm.Status.Phase {
		case "":
			usageMap[vm.GetName()] = false
		case v1alpha2.MachinePending:
			usageMap[vm.GetName()] = true
		case v1alpha2.MachineStopped:
			vmIsActive, err := h.isVMActive(ctx, vm)
			if err != nil {
				return nil, err
			}

			usageMap[vm.GetName()] = vmIsActive
		default:
			usageMap[vm.GetName()] = true
		}
	}

	return usageMap, nil
}

func (h InUseHandler) isVMActive(ctx context.Context, vm v1alpha2.VirtualMachine) (bool, error) {
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

	return false, nil
}

func (h InUseHandler) updateAttachedVirtualMachinesStatus(vd *v1alpha2.VirtualDisk, usageMap map[string]bool) {
	currentlyMountedVM := commonvd.GetCurrentlyMountedVMName(vd)

	attachedVMs := make([]v1alpha2.AttachedVirtualMachine, 0, len(usageMap))
	setAnyToTrue := false

	if used, exists := usageMap[currentlyMountedVM]; exists && used {
		for key := range usageMap {
			if key == currentlyMountedVM {
				attachedVMs = append(attachedVMs, v1alpha2.AttachedVirtualMachine{
					Name:    key,
					Mounted: true,
				})
			} else {
				attachedVMs = append(attachedVMs, v1alpha2.AttachedVirtualMachine{
					Name:    key,
					Mounted: false,
				})
			}
		}
	} else {
		for key, value := range usageMap {
			if !setAnyToTrue && value {
				attachedVMs = append(attachedVMs, v1alpha2.AttachedVirtualMachine{
					Name:    key,
					Mounted: true,
				})
				setAnyToTrue = true
			} else {
				attachedVMs = append(attachedVMs, v1alpha2.AttachedVirtualMachine{
					Name:    key,
					Mounted: false,
				})
			}
		}
	}

	vd.Status.AttachedToVirtualMachines = attachedVMs
}

func (h InUseHandler) checkUsageByVM(vd *v1alpha2.VirtualDisk) bool {
	for _, attachedVM := range vd.Status.AttachedToVirtualMachines {
		if attachedVM.Mounted {
			return true
		}
	}

	return false
}

func (h InUseHandler) checkUsageByVI(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	var vis v1alpha2.VirtualImageList
	err := h.client.List(ctx, &vis, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return false, fmt.Errorf("error getting virtual images: %w", err)
	}

	for _, vi := range vis.Items {
		if slices.Contains(imagePhasesUsingDisk, vi.Status.Phase) &&
			vi.Spec.DataSource.Type == v1alpha2.DataSourceTypeObjectRef &&
			vi.Spec.DataSource.ObjectRef != nil &&
			vi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskKind &&
			vi.Spec.DataSource.ObjectRef.Name == vd.Name {
			return true, nil
		}
	}

	return false, nil
}

func (h InUseHandler) checkUsageByCVI(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	var cvis v1alpha2.ClusterVirtualImageList
	err := h.client.List(ctx, &cvis, &client.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting cluster virtual images: %w", err)
	}
	for _, cvi := range cvis.Items {
		if slices.Contains(imagePhasesUsingDisk, cvi.Status.Phase) &&
			cvi.Spec.DataSource.Type == v1alpha2.DataSourceTypeObjectRef &&
			cvi.Spec.DataSource.ObjectRef != nil &&
			cvi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskKind &&
			cvi.Spec.DataSource.ObjectRef.Name == vd.Name {
			return true, nil
		}
	}

	return false, nil
}
