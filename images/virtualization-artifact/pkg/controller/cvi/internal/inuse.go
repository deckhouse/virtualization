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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(cvicondition.InUse).Generation(cvi.Generation)
	readyCondition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	cvi.Status.UsedInNamespaces = []string{}
	if readyCondition.Status == metav1.ConditionFalse && conditions.IsLastUpdated(readyCondition, cvi) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.NotInUse).
			Message("")
		conditions.SetCondition(cb, &cvi.Status.Conditions)
		return reconcile.Result{}, nil
	}
	if readyCondition.Status == metav1.ConditionUnknown || !conditions.IsLastUpdated(readyCondition, cvi) {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
		conditions.SetCondition(cb, &cvi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	vms, err := h.listVMsUsingImage(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	vmbdas, err := h.listVMBDAsUsingImage(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	vds, err := h.listVDsUsingImage(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	vis, err := h.listVIsUsingImage(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	cvis, err := h.listCVIsUsingImage(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	consumerCount := len(vms) + len(vds) + len(vis) + len(cvis)

	if consumerCount > 0 {
		cvi.Status.UsedInNamespaces = h.extractNamespacesFromObjects(vms, vmbdas, vds, vis)
		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.InUse).
			Message("")
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.NotInUse).
			Message("")
	}

	conditions.SetCondition(cb, &cvi.Status.Conditions)
	return reconcile.Result{}, nil
}

func (h InUseHandler) listVMsUsingImage(ctx context.Context, cvi *virtv2.ClusterVirtualImage) ([]client.Object, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms)
	if err != nil {
		return []client.Object{}, err
	}

	var vmsUsingImage []client.Object
	for _, vm := range vms.Items {
		if vm.Status.Phase == virtv2.MachineStopped {
			continue
		}

		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.ClusterVirtualImageKind && bd.Name == cvi.Name {
				vmsUsingImage = append(vmsUsingImage, &vm)
			}
		}
	}

	return vmsUsingImage, nil
}

func (h InUseHandler) listVMBDAsUsingImage(ctx context.Context, cvi *virtv2.ClusterVirtualImage) ([]client.Object, error) {
	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err := h.client.List(ctx, &vmbdas)
	if err != nil {
		return []client.Object{}, err
	}

	var vmbdasUsedImage []client.Object
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.BlockDeviceRef.Kind == virtv2.ClusterVirtualImageKind && vmbda.Spec.BlockDeviceRef.Name == cvi.Name {
			vmbdasUsedImage = append(vmbdasUsedImage, &vmbda)
		}
	}

	return vmbdasUsedImage, nil
}

func (h InUseHandler) listVDsUsingImage(ctx context.Context, cvi *virtv2.ClusterVirtualImage) ([]client.Object, error) {
	var vds virtv2.VirtualDiskList
	err := h.client.List(ctx, &vds, client.MatchingFields{
		indexer.IndexFieldVDByCVIDataSource: cvi.GetName(),
	})
	if err != nil {
		return []client.Object{}, err
	}

	var vdsNotReady []client.Object
	for _, vd := range vds.Items {
		phase := vd.Status.Phase
		isProvisioning := (phase == virtv2.DiskPending) ||
			(phase == virtv2.DiskProvisioning) ||
			(phase == virtv2.DiskWaitForFirstConsumer) ||
			(phase == virtv2.DiskFailed)

		if isProvisioning {
			vdsNotReady = append(vdsNotReady, &vd)
		}
	}

	return vdsNotReady, nil
}

func (h InUseHandler) listVIsUsingImage(ctx context.Context, cvi *virtv2.ClusterVirtualImage) ([]client.Object, error) {
	var vis virtv2.VirtualImageList
	err := h.client.List(ctx, &vis, client.MatchingFields{
		indexer.IndexFieldVIByCVIDataSource: cvi.GetName(),
	})
	if err != nil {
		return []client.Object{}, err
	}

	var visNotReady []client.Object
	for _, vi := range vis.Items {
		phase := vi.Status.Phase
		isProvisioning := (phase == virtv2.ImagePending) || (phase == virtv2.ImageProvisioning) || (phase == virtv2.ImageFailed)

		if isProvisioning {
			visNotReady = append(visNotReady, &vi)
		}
	}

	return visNotReady, nil
}

func (h InUseHandler) listCVIsUsingImage(ctx context.Context, cvi *virtv2.ClusterVirtualImage) ([]client.Object, error) {
	var cvis virtv2.ClusterVirtualImageList
	err := h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByCVIDataSource: cvi.GetName(),
	})
	if err != nil {
		return []client.Object{}, err
	}

	var cvisNotReady []client.Object
	for _, cviItem := range cvis.Items {
		phase := cviItem.Status.Phase
		isProvisioning := (phase == virtv2.ImagePending) || (phase == virtv2.ImageProvisioning) || (phase == virtv2.ImageFailed)

		if isProvisioning {
			cvisNotReady = append(cvisNotReady, &cviItem)
		}
	}

	return cvisNotReady, nil
}

func (h InUseHandler) extractNamespacesFromObjects(vms, vmbdas, vds, vis []client.Object) []string {
	var objects []client.Object
	objects = append(objects, vms...)
	objects = append(objects, vmbdas...)
	objects = append(objects, vds...)
	objects = append(objects, vis...)

	var namespaces []string
	namespacesMap := make(map[string]struct{})
	for _, obj := range objects {
		namespace := obj.GetNamespace()
		if namespace == "" {
			namespace = "default"
		}

		_, ok := namespacesMap[namespace]
		if !ok {
			namespaces = append(namespaces, namespace)
			namespacesMap[namespace] = struct{}{}
		}
	}

	return namespaces
}
