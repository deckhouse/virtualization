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

	var vmUsedImage []client.Object
	for _, vm := range vms.Items {
		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.VirtualImageKind && bd.Name == vi.Name {
				vmUsedImage = append(vmUsedImage, &vm)
				break
			}
		}
	}

	var vds virtv2.VirtualDiskList
	err = h.client.List(ctx, &vds, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVDByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	var vdsNotReady []client.Object
	for _, vd := range vds.Items {
		if vd.Status.Phase != virtv2.DiskReady {
			vdsNotReady = append(vdsNotReady, &vd)
		}
	}

	var vis virtv2.VirtualImageList
	err = h.client.List(ctx, &vis, client.InNamespace(vi.GetNamespace()), client.MatchingFields{
		indexer.IndexFieldVIByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	var visNotReady []client.Object
	for _, vi := range vis.Items {
		if vi.Status.Phase != virtv2.ImageReady {
			visNotReady = append(visNotReady, &vi)
		}
	}

	var cvis virtv2.ClusterVirtualImageList
	err = h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByVIDataSource: vi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	var cvisFiltered []client.Object
	for _, cvi := range cvis.Items {
		if cvi.Spec.DataSource.ObjectRef == nil || cvi.Status.Phase == virtv2.ImageReady {
			continue
		}
		if cvi.Spec.DataSource.ObjectRef.Namespace == vi.GetNamespace() {
			cvisFiltered = append(cvisFiltered, &cvi)
		}
	}

	consumerCount := len(vmUsedImage) + len(vdsNotReady) + len(visNotReady) + len(cvisFiltered)

	if consumerCount > 0 {
		var msgs []string
		if len(vmUsedImage) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualMachineKind, vmUsedImage...))
		}

		if len(vdsNotReady) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualDiskKind, vdsNotReady...))
		}

		if len(visNotReady) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualImageKind, visNotReady...))
		}

		if len(cvisFiltered) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.ClusterVirtualImageKind, cvisFiltered...))
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.InUse).
			Message(service.CapitalizeFirstLetter(fmt.Sprintf("%s.", strings.Join(msgs, ", "))))
		conditions.SetCondition(cb, &vi.Status.Conditions)
	} else {
		conditions.RemoveCondition(vicondition.InUse, &vi.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func (h InUseHandler) Name() string {
	return inUseHandlerName
}

func getTerminationMessage(objectKind string, objects ...client.Object) string {
	var objectFilteredNames []string
	for _, obj := range objects {
		if obj.GetObjectKind().GroupVersionKind().Kind == objectKind {
			objectFilteredNames = append(objectFilteredNames, obj.GetName())
		}
	}

	if objectKind == virtv2.VirtualMachineKind {
		switch len(objectFilteredNames) {
		case 1:
			return fmt.Sprintf("the VirtualImage is currently attached to the VirtualMachine %s", objectFilteredNames[0])
		case 2, 3:
			return fmt.Sprintf("the VirtualImage is currently attached to the VirtualMachines: %s", strings.Join(objectFilteredNames, ", "))
		default:
			return fmt.Sprintf("%d VirtualMachines are using the VirtualImage", len(objectFilteredNames))
		}
	} else {
		switch len(objectFilteredNames) {
		case 1:
			return fmt.Sprintf("the VirtualImage is currently being used to create the %s %s", objectKind, objectFilteredNames[0])
		case 2, 3:
			return fmt.Sprintf("the VirtualImage is currently being used to create the %ss: %s", objectKind, strings.Join(objectFilteredNames, ", "))
		default:
			return fmt.Sprintf("the VirtualImage is currently used to create %d %ss", len(objectFilteredNames), objectKind)
		}
	}
}
