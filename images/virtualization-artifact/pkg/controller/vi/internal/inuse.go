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
	if readyCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(readyCondition, vi) {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.NotInUse).
			Message("")

		conditions.SetCondition(cb, &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	vms, err := h.listVMsUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}

	vds, err := h.listVDsUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}

	vis, err := h.listVIsUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}

	cvis, err := h.listCVIsUsingImage(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}

	consumerCount := len(vms) + len(vds) + len(vis) + len(cvis)

	if consumerCount > 0 {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.InUse).
			Message("")
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.NotInUse).
			Message("")
	}

	conditions.SetCondition(cb, &vi.Status.Conditions)
	return reconcile.Result{}, nil
}

func (h InUseHandler) Name() string {
	return inUseHandlerName
}

func (h InUseHandler) listVMsUsingImage(ctx context.Context, vi *virtv2.VirtualImage) ([]client.Object, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, client.InNamespace(vi.GetNamespace()))
	if err != nil {
		return []client.Object{}, err
	}

	var vmsUsingImage []client.Object
	for _, vm := range vms.Items {
		if vm.Status.Phase == virtv2.MachineStopped {
			continue
		}

		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.VirtualImageKind && bd.Name == vi.Name {
				vmsUsingImage = append(vmsUsingImage, &vm)
				break
			}
		}
	}

	return vmsUsingImage, nil
}

func (h InUseHandler) listVDsUsingImage(ctx context.Context, vi *virtv2.VirtualImage) ([]client.Object, error) {
	var vds virtv2.VirtualDiskList
	err := h.client.List(ctx, &vds, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVDByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return []client.Object{}, err
	}

	var vdsNotReady []client.Object
	for _, vd := range vds.Items {
		if vd.Status.Phase != virtv2.DiskReady && vd.Status.Phase != virtv2.DiskTerminating {
			vdsNotReady = append(vdsNotReady, &vd)
		}
	}

	return vdsNotReady, nil
}

func (h InUseHandler) listVIsUsingImage(ctx context.Context, vi *virtv2.VirtualImage) ([]client.Object, error) {
	var vis virtv2.VirtualImageList
	err := h.client.List(ctx, &vis, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVIByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return []client.Object{}, err
	}

	var visNotReady []client.Object
	for _, viItem := range vis.Items {
		if viItem.Status.Phase != virtv2.ImageReady && viItem.Status.Phase != virtv2.ImageTerminating {
			visNotReady = append(visNotReady, &viItem)
		}
	}

	return visNotReady, nil
}

func (h InUseHandler) listCVIsUsingImage(ctx context.Context, vi *virtv2.VirtualImage) ([]client.Object, error) {
	var cvis virtv2.ClusterVirtualImageList
	err := h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return []client.Object{}, err
	}

	var cvisFiltered []client.Object
	for _, cvi := range cvis.Items {
		if cvi.Spec.DataSource.ObjectRef == nil || cvi.Status.Phase == virtv2.ImageReady || cvi.Status.Phase == virtv2.ImageTerminating {
			continue
		}
		if cvi.Spec.DataSource.ObjectRef.Namespace == vi.GetNamespace() {
			cvisFiltered = append(cvisFiltered, &cvi)
		}
	}

	return cvisFiltered, nil
}
