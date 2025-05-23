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

package reconciler

import (
	"context"
	"fmt"
	"maps"
	"reflect"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ResourceObject[T, ST any] interface {
	comparable
	client.Object
	DeepCopy() T
	GetObjectMeta() metav1.Object
}

type ObjectStatusGetter[T, ST any] func(obj T) ST

type ObjectFactory[T any] func() T

type Resource[T ResourceObject[T, ST], ST any] struct {
	name       types.NamespacedName
	currentObj T
	changedObj T
	emptyObj   T

	objFactory      ObjectFactory[T]
	objStatusGetter ObjectStatusGetter[T, ST]
	client          client.Client
}

func NewResource[T ResourceObject[T, ST], ST any](name types.NamespacedName, client client.Client, objFactory ObjectFactory[T], objStatusGetter ObjectStatusGetter[T, ST]) *Resource[T, ST] {
	return &Resource[T, ST]{
		name:            name,
		client:          client,
		objFactory:      objFactory,
		objStatusGetter: objStatusGetter,
	}
}

func (r *Resource[T, ST]) getObjStatus(obj T) (ret ST) {
	if obj != r.emptyObj {
		ret = r.objStatusGetter(obj)
	}
	return
}

func (r *Resource[T, ST]) Name() types.NamespacedName {
	return r.name
}

func (r *Resource[T, ST]) Fetch(ctx context.Context) error {
	currentObj, err := object.FetchObject(ctx, r.name, r.client, r.objFactory())
	if err != nil {
		return err
	}

	r.currentObj = currentObj
	if r.IsEmpty() {
		r.changedObj = r.emptyObj
		return nil
	}

	r.changedObj = currentObj.DeepCopy()
	return nil
}

func (r *Resource[T, ST]) IsEmpty() bool {
	return r.currentObj == r.emptyObj
}

func (r *Resource[T, ST]) Current() T {
	return r.currentObj
}

func (r *Resource[T, ST]) Changed() T {
	return r.changedObj
}

// rewriteObject is part of the transition from version 1.14, where you can specify empty reasons. After version 1.15, this feature is not needed.
// TODO: Delete me after release v1.15
func rewriteObject(obj client.Object) {
	var conds []metav1.Condition

	switch obj.GetObjectKind().GroupVersionKind().Kind {
	case virtv2.VirtualMachineKind:
		vm := obj.(*virtv2.VirtualMachine)
		conds = vm.Status.Conditions
	case virtv2.VirtualDiskKind:
		vd := obj.(*virtv2.VirtualDisk)
		conds = vd.Status.Conditions
	case virtv2.VirtualImageKind:
		vi := obj.(*virtv2.VirtualImage)
		conds = vi.Status.Conditions
	case virtv2.ClusterVirtualImageKind:
		cvi := obj.(*virtv2.ClusterVirtualImage)
		conds = cvi.Status.Conditions
	case virtv2.VirtualMachineBlockDeviceAttachmentKind:
		vmbda := obj.(*virtv2.VirtualMachineBlockDeviceAttachment)
		conds = vmbda.Status.Conditions
	case virtv2.VirtualMachineIPAddressKind:
		ip := obj.(*virtv2.VirtualMachineIPAddress)
		conds = ip.Status.Conditions
	case virtv2.VirtualMachineIPAddressLeaseKind:
		ipl := obj.(*virtv2.VirtualMachineIPAddressLease)
		conds = ipl.Status.Conditions
	case virtv2.VirtualMachineOperationKind:
		vmop := obj.(*virtv2.VirtualMachineOperation)
		conds = vmop.Status.Conditions
	case virtv2.VirtualDiskSnapshotKind:
		snap := obj.(*virtv2.VirtualDiskSnapshot)
		conds = snap.Status.Conditions
	case virtv2.VirtualMachineClassKind:
		class := obj.(*virtv2.VirtualMachineClass)
		conds = class.Status.Conditions
	case virtv2.VirtualMachineRestoreKind:
		restore := obj.(*virtv2.VirtualMachineRestore)
		conds = restore.Status.Conditions
	case virtv2.VirtualMachineSnapshotKind:
		snap := obj.(*virtv2.VirtualMachineSnapshot)
		conds = snap.Status.Conditions
	}

	rewriteConditions(conds)
}

func rewriteConditions(conds []metav1.Condition) {
	for i := range conds {
		if conds[i].Reason == "" {
			conds[i].Reason = conditions.ReasonUnknown.String()
		}
		if conds[i].Status == "" {
			conds[i].Status = metav1.ConditionUnknown
		}
	}
}

func (r *Resource[T, ST]) Update(ctx context.Context) error {
	if r.IsEmpty() {
		return nil
	}

	rewriteObject(r.changedObj)

	if !reflect.DeepEqual(r.getObjStatus(r.currentObj), r.getObjStatus(r.changedObj)) {
		// Save some metadata fields.
		finalizers := r.changedObj.GetFinalizers()
		labels := r.changedObj.GetLabels()
		annotations := r.changedObj.GetAnnotations()
		if err := r.client.Status().Update(ctx, r.changedObj); err != nil {
			return fmt.Errorf("error updating status subresource: %w", err)
		}
		// Restore metadata in changedObject.
		r.changedObj.SetFinalizers(finalizers)
		r.changedObj.SetLabels(labels)
		r.changedObj.SetAnnotations(annotations)
	}

	metadataPatch := patch.NewJsonPatch()

	if !slices.Equal(r.currentObj.GetFinalizers(), r.changedObj.GetFinalizers()) {
		metadataPatch.Append(r.JSONPatchOpsForFinalizers()...)
	}
	if !maps.Equal(r.currentObj.GetAnnotations(), r.changedObj.GetAnnotations()) {
		metadataPatch.Append(r.JSONPatchOpsForAnnotations()...)
	}
	if !maps.Equal(r.currentObj.GetLabels(), r.changedObj.GetLabels()) {
		metadataPatch.Append(r.JSONPatchOpsForLabels()...)
	}

	if metadataPatch.Len() == 0 {
		return nil
	}

	metadataPatchBytes, err := metadataPatch.Bytes()
	if err != nil {
		return err
	}
	jsonPatch := client.RawPatch(types.JSONPatchType, metadataPatchBytes)
	if err = r.client.Patch(ctx, r.changedObj, jsonPatch); err != nil {
		if r.changedObj.GetDeletionTimestamp() != nil && len(r.changedObj.GetFinalizers()) == 0 && kerrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("error patching metadata (%s): %w", string(metadataPatchBytes), err)
	}

	return nil
}

func (r *Resource[T, ST]) JSONPatchOpsForFinalizers() []patch.JsonPatchOperation {
	return []patch.JsonPatchOperation{
		patch.NewJsonPatchOperation(patch.PatchReplaceOp, "/metadata/finalizers", r.changedObj.GetFinalizers()),
	}
}

func (r *Resource[T, ST]) JSONPatchOpsForAnnotations() []patch.JsonPatchOperation {
	return []patch.JsonPatchOperation{
		patch.NewJsonPatchOperation(patch.PatchTestOp, "/metadata/annotations", r.currentObj.GetAnnotations()),
		patch.NewJsonPatchOperation(patch.PatchReplaceOp, "/metadata/annotations", r.changedObj.GetAnnotations()),
	}
}

func (r *Resource[T, ST]) JSONPatchOpsForLabels() []patch.JsonPatchOperation {
	return []patch.JsonPatchOperation{
		patch.NewJsonPatchOperation(patch.PatchTestOp, "/metadata/labels", r.currentObj.GetLabels()),
		patch.NewJsonPatchOperation(patch.PatchReplaceOp, "/metadata/labels", r.changedObj.GetLabels()),
	}
}

func MergeResults(results ...reconcile.Result) reconcile.Result {
	var result reconcile.Result
	for _, r := range results {
		if r.IsZero() {
			continue
		}
		if r.Requeue && r.RequeueAfter == 0 {
			return r
		}
		if result.IsZero() && r.RequeueAfter > 0 {
			result = r
			continue
		}
		if r.RequeueAfter > 0 && r.RequeueAfter < result.RequeueAfter {
			result.RequeueAfter = r.RequeueAfter
		}
	}
	return result
}
