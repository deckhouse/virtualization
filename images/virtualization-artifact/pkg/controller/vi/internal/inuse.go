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

package internal

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

const inUseHandlerName = "InUseHandler"

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vicondition.InUse).Generation(vi.Generation)
	readyCondition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	if readyCondition.Status == metav1.ConditionFalse &&
		readyCondition.Reason != vicondition.Lost.String() &&
		conditions.IsLastUpdated(readyCondition, vi) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.NotInUse).
			Message("")

		conditions.SetCondition(cb, &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}
	if readyCondition.Status == metav1.ConditionUnknown || !conditions.IsLastUpdated(readyCondition, vi) {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")

		conditions.SetCondition(cb, &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	hasVM, err := h.hasVMUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasVM {
		setInUseConditionTrue(vi, cb)
		return reconcile.Result{}, nil
	}

	hasVMBDA, err := h.hasVMBDAsUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasVMBDA {
		setInUseConditionTrue(vi, cb)
		return reconcile.Result{}, nil
	}

	hasVD, err := h.hasVDUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasVD {
		setInUseConditionTrue(vi, cb)
		return reconcile.Result{}, nil
	}

	hasVI, err := h.hasVIUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasVI {
		setInUseConditionTrue(vi, cb)
		return reconcile.Result{}, nil
	}

	hasCVI, err := h.hasCVIUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasCVI {
		setInUseConditionTrue(vi, cb)
		return reconcile.Result{}, nil
	}

	cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.NotInUse).
		Message("")
	conditions.SetCondition(cb, &vi.Status.Conditions)
	return reconcile.Result{}, nil
}

func (h InUseHandler) Name() string {
	return inUseHandlerName
}

func (h InUseHandler) hasVMUsingImage(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, client.InNamespace(vi.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, vm := range vms.Items {
		if vm.Status.Phase == virtv2.MachineStopped {
			continue
		}

		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.VirtualImageKind && bd.Name == vi.Name {
				return true, nil
			}
		}
	}

	return false, nil
}

func (h InUseHandler) hasVMBDAsUsingImage(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err := h.client.List(ctx, &vmbdas, client.InNamespace(vi.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.BlockDeviceRef.Kind == virtv2.VMBDAObjectRefKindVirtualImage && vmbda.Spec.BlockDeviceRef.Name == vi.Name {
			return true, nil
		}
	}

	return false, nil
}

func (h InUseHandler) hasVDUsingImage(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	var vds virtv2.VirtualDiskList
	err := h.client.List(ctx, &vds, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVDByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return false, err
	}

	for _, vd := range vds.Items {
		phase := vd.Status.Phase
		isProvisioning := (phase == virtv2.DiskPending) ||
			(phase == virtv2.DiskProvisioning) ||
			(phase == virtv2.DiskWaitForFirstConsumer) ||
			(phase == virtv2.DiskFailed)

		if isProvisioning {
			return true, nil
		}
	}

	return false, nil
}

func (h InUseHandler) hasVIUsingImage(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	var vis virtv2.VirtualImageList
	err := h.client.List(ctx, &vis, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVIByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return false, err
	}

	for _, viItem := range vis.Items {
		phase := viItem.Status.Phase
		isProvisioning := (phase == virtv2.ImagePending) || (phase == virtv2.ImageProvisioning) || (phase == virtv2.ImageFailed)
		if isProvisioning {
			return true, nil
		}
	}

	return false, nil
}

func (h InUseHandler) hasCVIUsingImage(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	var cvis virtv2.ClusterVirtualImageList
	err := h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return false, err
	}

	for _, cvi := range cvis.Items {
		if cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		phase := cvi.Status.Phase
		isProvisioning := (phase == virtv2.ImagePending) || (phase == virtv2.ImageProvisioning) || (phase == virtv2.ImageFailed)
		if !isProvisioning {
			continue
		}

		if cvi.Spec.DataSource.ObjectRef.Namespace == vi.GetNamespace() {
			return true, nil
		}
	}

	return false, nil
}

func setInUseConditionTrue(vi *virtv2.VirtualImage, cb *conditions.ConditionBuilder) {
	cb.
		Status(metav1.ConditionTrue).
		Reason(vicondition.InUse).
		Message("")

	conditions.SetCondition(cb, &vi.Status.Conditions)
}
