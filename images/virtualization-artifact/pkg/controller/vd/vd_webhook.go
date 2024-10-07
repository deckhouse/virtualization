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
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/validators"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskValidator interface {
	ValidateCreate(ctx context.Context, vm *virtv2.VirtualDisk) (admission.Warnings, error)
	ValidateUpdate(ctx context.Context, oldVM, newVM *virtv2.VirtualDisk) (admission.Warnings, error)
}

type Validator struct {
	validators []VirtualDiskValidator
	logger     *slog.Logger
}

func NewValidator(client client.Client, logger *slog.Logger) *Validator {
	return &Validator{
		validators: []VirtualDiskValidator{
			validators.NewPVCSizeValidator(client),
			validators.NewSpecChangesValidator(),
			validators.NewISOSourceValidator(client),
		},
		logger: logger.With("webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vd, ok := obj.(*virtv2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDisk but got a %T", obj)
	}

	log := logger.FromContext(ctx)
	log.Info("[AAAAAAAAAAA] Validating virtual disk", "spec.pvc.size", vd.Spec.PersistentVolumeClaim.Size)

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateCreate(ctx, vd)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
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

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateUpdate(ctx, oldVD, newVD)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
