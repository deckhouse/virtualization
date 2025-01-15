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

package validator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type CreateValidator[T client.Object] interface {
	ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error)
}

type UpdateValidator[T client.Object] interface {
	ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error)
}

type DeleteValidator[T client.Object] interface {
	ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error)
}

type (
	PredicateCreateFunc[T client.Object] func(obj T) bool
	PredicateUpdateFunc[T client.Object] func(oldObj, newObj T) bool
	PredicateDeleteFunc[T client.Object] func(obj T) bool
)

type Predicate[T client.Object] struct {
	Create PredicateCreateFunc[T]
	Update PredicateUpdateFunc[T]
	Delete PredicateDeleteFunc[T]
}

var _ admission.CustomValidator = &Validator[client.Object]{}

type Validator[T client.Object] struct {
	create []CreateValidator[T]
	update []UpdateValidator[T]
	delete []DeleteValidator[T]

	predicate *Predicate[T]

	log *log.Logger
}

func NewValidator[T client.Object](log *log.Logger) *Validator[T] {
	return &Validator[T]{log: log}
}

func (v *Validator[T]) WithCreateValidators(validators ...CreateValidator[T]) *Validator[T] {
	v.create = append(v.create, validators...)
	return v
}

func (v *Validator[T]) WithUpdateValidators(validators ...UpdateValidator[T]) *Validator[T] {
	v.update = append(v.update, validators...)
	return v
}

func (v *Validator[T]) WithDeleteValidators(validators ...DeleteValidator[T]) *Validator[T] {
	v.delete = append(v.delete, validators...)
	return v
}

func (v *Validator[T]) WithPredicate(predicate *Predicate[T]) *Validator[T] {
	v.predicate = predicate
	return v
}

func (v *Validator[T]) ValidateCreate(ctx context.Context, obj runtime.Object) (w admission.Warnings, err error) {
	if len(v.create) == 0 {
		err = fmt.Errorf("misconfigured webhook rules: create operation not implemented")
		v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", logger.SlogErr(err))
		return nil, nil
	}
	defer func() {
		if err != nil {
			v.logErr(err)
		}
	}()

	o, err := v.newObject(obj)
	if err != nil {
		return nil, err
	}
	if !v.needCreateValidate(o) {
		return nil, nil
	}

	var warnings admission.Warnings

	for _, validator := range v.create {
		warn, err := validator.ValidateCreate(ctx, o)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (w admission.Warnings, err error) {
	if len(v.update) == 0 {
		err = fmt.Errorf("misconfigured webhook rules: update operation not implemented")
		v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", logger.SlogErr(err))
		return nil, nil
	}
	defer func() {
		if err != nil {
			v.logErr(err)
		}
	}()

	oldO, err := v.newObject(oldObj)
	if err != nil {
		return nil, err
	}
	newO, err := v.newObject(newObj)
	if err != nil {
		return nil, err
	}
	if !v.needUpdateValidate(oldO, newO) {
		return nil, nil
	}
	var warnings admission.Warnings

	for _, validator := range v.update {
		warn, err := validator.ValidateUpdate(ctx, oldO, newO)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator[T]) ValidateDelete(ctx context.Context, obj runtime.Object) (w admission.Warnings, err error) {
	if len(v.delete) == 0 {
		err = fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
		v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", logger.SlogErr(err))
		return nil, nil
	}
	defer func() {
		if err != nil {
			v.logErr(err)
		}
	}()

	o, err := v.newObject(obj)
	if err != nil {
		return nil, err
	}
	if !v.needDeleteValidate(o) {
		return nil, nil
	}

	var warnings admission.Warnings

	for _, validator := range v.delete {
		warn, err := validator.ValidateDelete(ctx, o)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator[T]) newObject(obj runtime.Object) (T, error) {
	newObj, ok := obj.(T)
	if !ok {
		var empty T
		return empty, fmt.Errorf("expected a new %T but got a %T", empty, newObj)
	}
	return newObj, nil
}

func (v *Validator[T]) needCreateValidate(obj T) bool {
	if v.predicate != nil && v.predicate.Create != nil {
		return v.predicate.Create(obj)
	}
	return true
}

func (v *Validator[T]) needUpdateValidate(oldObj, newObj T) bool {
	if v.predicate != nil && v.predicate.Update != nil {
		return v.predicate.Update(oldObj, newObj)
	}
	return true
}

func (v *Validator[T]) needDeleteValidate(obj T) bool {
	if v.predicate != nil && v.predicate.Delete != nil {
		return v.predicate.Delete(obj)
	}
	return true
}

func (v *Validator[T]) logErr(err error) {
	var empty T
	v.log.Error(
		fmt.Sprintf("An error occurred while processing %T", empty),
		logger.SlogErr(err),
	)
}
