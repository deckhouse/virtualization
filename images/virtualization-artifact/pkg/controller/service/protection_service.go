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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/common/resource_builder"
)

type ProtectionService struct {
	client    client.Client
	finalizer string
}

func NewProtectionService(client client.Client, finalizer string) *ProtectionService {
	return &ProtectionService{
		client:    client,
		finalizer: finalizer,
	}
}

func (s ProtectionService) AddOwnerRef(ctx context.Context, owner client.Object, objs ...client.Object) error {
	if owner == nil || reflect.ValueOf(owner).IsNil() {
		return nil
	}

	ownerRef := MakeOwnerReference(owner)

	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}

		if resource_builder.SetOwnerRef(obj, ownerRef) {
			patch, err := GetPatchOwnerReferences(obj.GetOwnerReferences())
			if err != nil {
				return err
			}

			err = s.client.Patch(ctx, obj, patch)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s ProtectionService) AddProtection(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}

		// No new finalizers can be added if the object is being deleted.
		if obj.GetDeletionTimestamp() != nil {
			continue
		}

		currentFinalizers := obj.GetFinalizers()
		if controllerutil.AddFinalizer(obj, s.finalizer) {
			patch, err := GetPatchFinalizers(currentFinalizers, obj.GetFinalizers())
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			if err != nil {
				return fmt.Errorf("failed to generate patch for %q, %q: %w", kind, obj.GetName(), err)
			}

			err = s.client.Patch(ctx, obj, patch)
			if err != nil {
				return fmt.Errorf("failed to add finalizer %q on the %q, %q: %w", s.finalizer, kind, obj.GetName(), err)
			}
		}
	}

	return nil
}

func (s ProtectionService) RemoveProtection(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}

		currentFinalizers := obj.GetFinalizers()
		if controllerutil.RemoveFinalizer(obj, s.finalizer) {
			patch, err := GetPatchFinalizers(currentFinalizers, obj.GetFinalizers())
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			if err != nil {
				return fmt.Errorf("failed to generate patch for %q, %q: %w", kind, obj.GetName(), err)
			}

			err = s.client.Patch(ctx, obj, patch)
			if err != nil {
				return fmt.Errorf("failed to remove finalizer %q on the %q, %q: %w", s.finalizer, kind, obj.GetName(), err)
			}
		}
	}

	return nil
}

func (s ProtectionService) GetFinalizer() string {
	return s.finalizer
}

func MakeOwnerReference(owner client.Object) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: owner.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       owner.GetObjectKind().GroupVersionKind().Kind,
		Name:       owner.GetName(),
		UID:        owner.GetUID(),
	}
}

func GetPatchOwnerReferences(ownerReferences []metav1.OwnerReference) (client.Patch, error) {
	data, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"ownerReferences": ownerReferences,
		},
	})
	if err != nil {
		return nil, err
	}

	return client.RawPatch(types.MergePatchType, data), nil
}

func GetPatchFinalizers(currentFinalizers, newFinalizers []string) (client.Patch, error) {
	metadataPatch := patch.NewJSONPatch()

	metadataPatch.Append(patch.NewJSONPatchOperation(patch.PatchTestOp, "/metadata/finalizers", currentFinalizers))
	metadataPatch.Append(patch.NewJSONPatchOperation(patch.PatchReplaceOp, "/metadata/finalizers", newFinalizers))

	metadataPatchBytes, err := metadataPatch.Bytes()
	if err != nil {
		return nil, err
	}

	return client.RawPatch(types.JSONPatchType, metadataPatchBytes), nil
}
