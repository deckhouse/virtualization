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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
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

func (h InUseHandler) Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	if cvi.DeletionTimestamp == nil {
		conditions.RemoveCondition(cvicondition.InUse, &cvi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	readyCondition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	if readyCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(readyCondition, cvi) {
		conditions.RemoveCondition(cvicondition.InUse, &cvi.Status.Conditions)
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(cvicondition.InUse).Generation(cvi.Generation)

	namespacesMap := make(map[string]struct{})

	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms)
	if err != nil {
		return reconcile.Result{}, err
	}

	var vmUsedImage []*virtv2.VirtualMachine
	for _, vm := range vms.Items {
		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.ClusterVirtualImageKind && bd.Name == cvi.Name {
				vmUsedImage = append(vmUsedImage, &vm)
				namespacesMap[vm.Namespace] = struct{}{}
			}
		}
	}

	var vds virtv2.VirtualDiskList
	err = h.client.List(ctx, &vds, client.MatchingFields{
		indexer.IndexFieldVDByCVIDataSourceNotReady: cvi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, vd := range vds.Items {
		namespacesMap[vd.Namespace] = struct{}{}
	}

	var vis virtv2.VirtualImageList
	err = h.client.List(ctx, &vis, client.MatchingFields{
		indexer.IndexFieldVIByCVIDataSourceNotReady: cvi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, vi := range vis.Items {
		namespacesMap[vi.Namespace] = struct{}{}
	}

	var cvis virtv2.ClusterVirtualImageList
	err = h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByCVIDataSourceNotReady: cvi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, cv := range cvis.Items {
		namespacesMap[cv.Namespace] = struct{}{}
	}

	consumerCount := len(vmUsedImage) + len(vds.Items) + len(vis.Items) + len(cvis.Items)

	if consumerCount > 0 {
		var messageBuilder strings.Builder
		var needComma bool
		if len(vmUsedImage) > 0 {
			needComma = true
			switch len(vmUsedImage) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("The ClusterVirtualImage is currently attached to the VirtualMachine %s/%s", vmUsedImage[0].Namespace, vmUsedImage[0].Name))
			case 2, 3:
				var vmNamespacedNames []string
				for _, vm := range vmUsedImage {
					vmNamespacedNames = append(vmNamespacedNames, fmt.Sprintf("%s/%s", vm.Namespace, vm.Name))
				}
				messageBuilder.WriteString(fmt.Sprintf("The ClusterVirtualImage is currently attached to the VirtualMachines: %s", strings.Join(vmNamespacedNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("%d VirtualMachines are using the ClusterVirtualImage", len(vmUsedImage)))
			}
		}

		if len(vds.Items) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}
			needComma = true

			switch len(vds.Items) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently being used to create the VirtualDisk %s/%s", vds.Items[0].Namespace, vds.Items[0].Name))
			case 2, 3:
				var vdNamespacedNames []string
				for _, vd := range vds.Items {
					vdNamespacedNames = append(vdNamespacedNames, fmt.Sprintf("%s/%s", vd.Namespace, vd.Name))
				}
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently being used to create the VirtualDisks: %s", strings.Join(vdNamespacedNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is used to create %d VirtualDisks", len(vds.Items)))
			}
		}

		if len(vis.Items) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}
			needComma = true

			switch len(vis.Items) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently being used to create the VirtualImage %s/%s", vis.Items[0].Namespace, vis.Items[0].Name))
			case 2, 3:
				var viNamespacedNames []string
				for _, vi := range vis.Items {
					viNamespacedNames = append(viNamespacedNames, fmt.Sprintf("%s/%s", vi.Namespace, vi.Name))
				}
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently being used to create the VirtualImages: %s", strings.Join(viNamespacedNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is used to create %d VirtualImages", len(vis.Items)))
			}
		}

		if len(cvis.Items) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}
			needComma = true

			switch len(cvis.Items) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently being used to create the ClusterVirtualImage %s", cvis.Items[0].Name))
			case 2, 3:
				var cviNames []string
				for _, cvi := range cvis.Items {
					cviNames = append(cviNames, cvi.Name)
				}
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently being used to create the ClusterVirtualImages: %s", strings.Join(cviNames, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is used to create %d ClusterVirtualImages", len(cvis.Items)))
			}
		}

		var namespaces []string
		for namespace := range namespacesMap {
			namespaces = append(namespaces, namespace)
		}

		if len(namespaces) > 0 {
			if needComma {
				messageBuilder.WriteString(", ")
			}

			switch len(namespaces) {
			case 1:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is currently using in Namespace %s", namespaces[0]))
			case 2, 3:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is using in Namespaces: %s", strings.Join(namespaces, ", ")))
			default:
				messageBuilder.WriteString(fmt.Sprintf("ClusterVirtualImage is using in %d Namespaces", len(namespaces)))
			}
		}

		cvi.Status.UsedInNamespaces = []string{}
		for namespace := range namespacesMap {
			cvi.Status.UsedInNamespaces = append(cvi.Status.UsedInNamespaces, namespace)
		}

		messageBuilder.WriteString(".")
		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.InUse).
			Message(service.CapitalizeFirstLetter(messageBuilder.String()))
		conditions.SetCondition(cb, &cvi.Status.Conditions)
	} else {
		conditions.RemoveCondition(cvicondition.InUse, &cvi.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func (h InUseHandler) Name() string {
	return inUseHandlerName
}
