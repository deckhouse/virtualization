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

package vd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type Validator struct {
	logger *slog.Logger
}

func NewValidator() *Validator {
	return &Validator{
		logger: slog.Default().With("controller", common.VDShortName, "webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	vd, ok := obj.(*virtv2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDisk but got a %T", obj)
	}

	v.logger.Info("Validating virtual disk", "spec.pvc.size", vd.Spec.PersistentVolumeClaim.Size)

	if vd.Spec.PersistentVolumeClaim.Size != nil && vd.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual machine disk size must be greater than 0")
	}

	if vd.Spec.DataSource == nil && (vd.Spec.PersistentVolumeClaim.Size == nil || vd.Spec.PersistentVolumeClaim.Size.IsZero()) {
		return nil, fmt.Errorf("if the data source is not specified, it's necessary to set spec.PersistentVolumeClaim.size to create blank virtual disk")
	}

	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newVD, ok := newObj.(*virtv2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDisk but got a %T", newObj)
	}

	oldVD, ok := oldObj.(*virtv2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualDisk but got a %T", oldObj)
	}

	v.logger.Info("Validating virtual disk",
		"old.spec.pvc.size", oldVD.Spec.PersistentVolumeClaim.Size,
		"new.spec.pvc.size", newVD.Spec.PersistentVolumeClaim.Size,
	)

	if newVD.Spec.PersistentVolumeClaim.Size == oldVD.Spec.PersistentVolumeClaim.Size {
		return nil, nil
	}

	if newVD.Spec.PersistentVolumeClaim.Size == nil {
		return nil, errors.New("spec.persistentVolumeClaim.size cannot be omitted once set")
	}

	if newVD.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual machine disk size must be greater than 0")
	}

	if oldVD.Spec.PersistentVolumeClaim.Size != nil && newVD.Spec.PersistentVolumeClaim.Size.Cmp(*oldVD.Spec.PersistentVolumeClaim.Size) == -1 {
		return nil, fmt.Errorf(
			"spec.persistentVolumeClaim.size value (%s) should be greater than or equal to the current value (%s)",
			newVD.Spec.PersistentVolumeClaim.Size.String(),
			oldVD.Spec.PersistentVolumeClaim.Size.String(),
		)
	}

	if oldVD.Generation == newVD.Generation {
		return nil, nil
	}

	ready, _ := service.GetCondition(cvicondition.ReadyType, newVD.Status.Conditions)
	if newVD.Status.Phase == virtv2.DiskReady || newVD.Status.Phase == virtv2.DiskLost || ready.Status == metav1.ConditionTrue {
		if !reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
			return nil, fmt.Errorf("VirtualDisk has already been created: data source cannot be changed after disk is created")
		}

		if !reflect.DeepEqual(oldVD.Spec.PersistentVolumeClaim.StorageClass, newVD.Spec.PersistentVolumeClaim.StorageClass) {
			return nil, fmt.Errorf("VirtualDisk has already been created: storage class cannot be changed after disk is created")
		}
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
