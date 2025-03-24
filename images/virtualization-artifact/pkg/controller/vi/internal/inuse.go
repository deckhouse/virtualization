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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
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
	if vi.DeletionTimestamp == nil {
		conditions.RemoveCondition(vicondition.InUse, &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	readyCondition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	if readyCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(readyCondition, vi) {
		conditions.RemoveCondition(vicondition.InUse, &vi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vicondition.InUse).Generation(vi.Generation)

	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, client.InNamespace(vi.GetNamespace()))
	if err != nil {
		return reconcile.Result{}, err
	}

	var vmUsedImage []*virtv2.VirtualMachine
	for _, vm := range vms.Items {
		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.VirtualImageKind && bd.Name == vi.Name {
				vmUsedImage = append(vmUsedImage, &vm)
			}
		}
	}

	var vds virtv2.VirtualDiskList
	err = h.client.List(ctx, &vds, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVDByVIDataSourceNotReady: vi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	var vis virtv2.VirtualImageList
	err = h.client.List(ctx, &vis, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVIByVIDataSourceNotReady: vi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	var cvis virtv2.ClusterVirtualImageList
	err = h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByVIDataSourceNotReady: vi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	var cvisFiltered []*virtv2.ClusterVirtualImage
	for _, cvi := range cvis.Items {
		if cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}
		if cvi.Spec.DataSource.ObjectRef.Namespace == vi.GetNamespace() {
			cvisFiltered = append(cvisFiltered, &cvi)
		}
	}

	consumerCount := len(vmUsedImage) + len(vds.Items) + len(vis.Items) + len(cvisFiltered)

	if consumerCount > 0 {
		var messageBuilder strings.Builder
		var needComma bool
		if len(vmUsedImage) > 0 {
			needComma = true
			switch len(vmUsedImage) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("The VirtualImage is currently attached to the VirtualMachine %s", vmUsedImage[0].Name))
			case 2, 3:
				var vmNames []string
				for _, vm := range vmUsedImage {
					vmNames = append(vmNames, vm.GetName())
				}
				messageBuilder.WriteString(fmt.Sprintf("The VirtualImage is currently attached to the VirtualMachines: %s", strings.Join(vmNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("%d VirtualMachines are using the VirtualImage", len(vmUsedImage)))
			}
		}

		if len(vds.Items) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}
			needComma = true

			switch len(vds.Items) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is currently being used to create the VirtualDisk %s", vds.Items[0].Name))
			case 2, 3:
				var vdNames []string
				for _, vd := range vds.Items {
					vdNames = append(vdNames, vd.GetName())
				}
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is currently being used to create the VirtualDisks: %s", strings.Join(vdNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is used to create %d VirtualDisks", len(vds.Items)))
			}
		}

		if len(vis.Items) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}
			needComma = true

			switch len(vis.Items) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is currently being used to create the VirtualImage %s", vis.Items[0].Name))
			case 2, 3:
				var viNames []string
				for _, vi := range vis.Items {
					viNames = append(viNames, vi.Name)
				}
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is currently being used to create the VirtualImages: %s", strings.Join(viNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is used to create %d VirtualImages", len(vis.Items)))
			}
		}

		if len(cvisFiltered) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}

			switch len(cvisFiltered) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is currently being used to create the ClusterVirtualImage %s", cvisFiltered[0].Name))
			case 2, 3:
				var cviNames []string
				for _, cvi := range cvisFiltered {
					cviNames = append(cviNames, cvi.Name)
				}
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is currently being used to create the ClusterVirtualImages: %s", strings.Join(cviNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("VirtualImage is used to create %d ClusterVirtualImages", len(cvisFiltered)))
			}
		}

		messageBuilder.WriteString(".")
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.InUse).
			Message(service.CapitalizeFirstLetter(messageBuilder.String()))
		conditions.SetCondition(cb, &vi.Status.Conditions)
	} else {
		conditions.RemoveCondition(vicondition.InUse, &vi.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func (h InUseHandler) Name() string {
	return inUseHandlerName
}
