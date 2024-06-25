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
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}

		err := controllerutil.SetOwnerReference(owner, obj, s.client.Scheme())
		if err != nil {
			return err
		}

		var patch client.Patch
		patch, err = GetPatchOwnerReferences(obj.GetOwnerReferences())
		if err != nil {
			return err
		}

		err = s.client.Patch(ctx, obj, patch)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s ProtectionService) AddProtection(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}

		if controllerutil.AddFinalizer(obj, s.finalizer) {
			patch, err := GetPatchFinalizers(obj.GetFinalizers())
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

func (s ProtectionService) RemoveProtection(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj).IsNil() {
			continue
		}

		if controllerutil.RemoveFinalizer(obj, s.finalizer) {
			patch, err := GetPatchFinalizers(obj.GetFinalizers())
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
