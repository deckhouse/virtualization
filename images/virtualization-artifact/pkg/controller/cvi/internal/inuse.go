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
	"k8s.io/apimachinery/pkg/types"
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

	var vmsUsingImage []client.Object
	for _, vm := range vms.Items {
		if vm.Status.Phase == virtv2.MachineStopped {
			continue
		}

		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == virtv2.ClusterVirtualImageKind && bd.Name == cvi.Name {
				vmsUsingImage = append(vmsUsingImage, &vm)
				namespacesMap[vm.Namespace] = struct{}{}
			}
		}
	}

	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err = h.client.List(ctx, &vmbdas)
	if err != nil {
		return reconcile.Result{}, err
	}

	var vmbdaUsedImage []client.Object
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.BlockDeviceRef.Kind == virtv2.ClusterVirtualImageKind && vmbda.Spec.BlockDeviceRef.Name == cvi.Name {
			vmbdaUsedImage = append(vmbdaUsedImage, &vmbda)
		}
	}

	var vds virtv2.VirtualDiskList
	err = h.client.List(ctx, &vds, client.MatchingFields{
		indexer.IndexFieldVDByCVIDataSource: cvi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	var vdsNotReady []client.Object
	for _, vd := range vds.Items {
		if vd.Status.Phase != virtv2.DiskReady && vd.Status.Phase != virtv2.DiskTerminating {
			vdsNotReady = append(vdsNotReady, &vd)
		}
	}

	for _, vd := range vdsNotReady {
		namespacesMap[vd.GetNamespace()] = struct{}{}
	}

	var vis virtv2.VirtualImageList
	err = h.client.List(ctx, &vis, client.MatchingFields{
		indexer.IndexFieldVIByCVIDataSource: cvi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	var visNotReady []client.Object
	for _, vi := range vis.Items {
		if vi.Status.Phase != virtv2.ImageReady && vi.Status.Phase != virtv2.ImageTerminating {
			visNotReady = append(visNotReady, &vi)
		}
	}

	for _, vi := range visNotReady {
		namespacesMap[vi.GetNamespace()] = struct{}{}
	}

	var cvis virtv2.ClusterVirtualImageList
	err = h.client.List(ctx, &cvis, client.MatchingFields{
		indexer.IndexFieldCVIByCVIDataSource: cvi.GetName(),
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	var cvisNotReady []client.Object
	for _, cviItem := range cvis.Items {
		if cviItem.Status.Phase != virtv2.ImageReady && cviItem.Status.Phase != virtv2.ImageTerminating {
			cvisNotReady = append(cvisNotReady, &cviItem)
		}
	}

	consumerCount := len(vmsUsingImage) + len(vdsNotReady) + len(visNotReady) + len(cvisNotReady)

	if consumerCount > 0 {
		var msgs []string
		if len(vmsUsingImage) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualMachineKind, vmsUsingImage...))
		}

		if len(vmbdaUsedImage) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualMachineBlockDeviceAttachmentKind, vmbdaUsedImage...))
		}

		if len(vdsNotReady) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualDiskKind, vdsNotReady...))
		}

		if len(visNotReady) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.VirtualImageKind, visNotReady...))
		}

		if len(cvisNotReady) > 0 {
			msgs = append(msgs, getTerminationMessage(virtv2.ClusterVirtualImageKind, cvisNotReady...))
		}

		var namespaces []string
		for namespace := range namespacesMap {
			namespaces = append(namespaces, namespace)
		}

		if len(namespaces) > 0 {
			msgs = append(msgs, getNamespacesTerminationMessage(namespaces))
		}

		cvi.Status.UsedInNamespaces = namespaces

		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.InUse).
			Message(service.CapitalizeFirstLetter(fmt.Sprintf("%s.", strings.Join(msgs, ", "))))
		conditions.SetCondition(cb, &cvi.Status.Conditions)
	} else {
		conditions.RemoveCondition(cvicondition.InUse, &cvi.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func (h InUseHandler) Name() string {
	return inUseHandlerName
}

func getTerminationMessage(objectKind string, objects ...client.Object) string {
	var objectFilteredNamespacedNames []types.NamespacedName
	for _, obj := range objects {
		if obj.GetObjectKind().GroupVersionKind().Kind == objectKind {
			objectFilteredNamespacedNames = append(objectFilteredNamespacedNames, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()})
		}
	}

	switch objectKind {
	case virtv2.VirtualMachineKind:
		return getVMTerminationMessage(objectFilteredNamespacedNames)
	case virtv2.VirtualMachineBlockDeviceAttachmentKind:
		return getVMBDATerminationMessage(objectFilteredNamespacedNames)
	default:
		return getDefaultTerminationMessage(objectKind, objectFilteredNamespacedNames)
	}
}

func nameSpacedNamesToStringSlice(namespacedNames []types.NamespacedName) []string {
	var result []string

	for _, namespacedName := range namespacedNames {
		if namespacedName.Namespace != "" {
			result = append(result, fmt.Sprintf("%s/%s", namespacedName.Namespace, namespacedName.Name))
		} else {
			result = append(result, namespacedName.Name)
		}
	}

	return result
}

func getDefaultTerminationMessage(objectKind string, objectNamespacedNames []types.NamespacedName) string {
	switch len(objectNamespacedNames) {
	case 1:
		if objectNamespacedNames[0].Namespace != "" {
			return fmt.Sprintf("the ClusterVirtualImage is currently being used to create the %s %s/%s", objectKind, objectNamespacedNames[0].Namespace, objectNamespacedNames[0].Name)
		} else {
			return fmt.Sprintf("the ClusterVirtualImage is currently being used to create the %s %s", objectKind, objectNamespacedNames[0].Name)
		}
	case 2, 3:
		return fmt.Sprintf("the ClusterVirtualImage is currently being used to create the %ss: %s", objectKind, strings.Join(nameSpacedNamesToStringSlice(objectNamespacedNames), ", "))
	default:
		return fmt.Sprintf("the ClusterVirtualImage is currently used to create %d %ss", len(objectNamespacedNames), objectKind)
	}
}

func getVMTerminationMessage(objectNamespacedNames []types.NamespacedName) string {
	switch len(objectNamespacedNames) {
	case 1:
		return fmt.Sprintf("the ClusterVirtualImage is currently attached to the VirtualMachine %s/%s", objectNamespacedNames[0].Namespace, objectNamespacedNames[0].Name)
	case 2, 3:
		return fmt.Sprintf("the ClusterVirtualImage is currently attached to the VirtualMachines: %s", strings.Join(nameSpacedNamesToStringSlice(objectNamespacedNames), ", "))
	default:
		return fmt.Sprintf("%d VirtualMachines are using the ClusterVirtualImage", len(objectNamespacedNames))
	}
}

func getVMBDATerminationMessage(objectNamespacedNames []types.NamespacedName) string {
	switch len(objectNamespacedNames) {
	case 1:
		if objectNamespacedNames[0].Namespace != "" {
			return fmt.Sprintf("the ClusterVirtualImage is currently being used by the VMBDA %s/%s", objectNamespacedNames[0].Namespace, objectNamespacedNames[0].Name)
		} else {
			return fmt.Sprintf("the ClusterVirtualImage is currently being used by the VMBDA %s", objectNamespacedNames[0].Name)
		}
	case 2, 3:
		return fmt.Sprintf("the ClusterVirtualImage is currently being used by the VMBDAs: %s", strings.Join(nameSpacedNamesToStringSlice(objectNamespacedNames), ", "))
	default:
		return fmt.Sprintf("the ClusterVirtualImage is currently used by %d VMBDAs", len(objectNamespacedNames))
	}
}

func getNamespacesTerminationMessage(namespaces []string) string {
	switch len(namespaces) {
	case 1:
		return fmt.Sprintf("the ClusterVirtualImage is currently using in Namespace %s", namespaces[0])
	case 2, 3:
		return fmt.Sprintf("the ClusterVirtualImage is currently using in Namespaces: %s", strings.Join(namespaces, ", "))
	default:
		return fmt.Sprintf("the ClusterVirtualImage is currently using in %d Namespaces", len(namespaces))
	}
}
