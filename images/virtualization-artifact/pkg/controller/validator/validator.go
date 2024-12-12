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

type Predicate[T client.Object] func(obj T) bool

var _ admission.CustomValidator = &Validator[client.Object]{}

type Validator[T client.Object] struct {
	create []CreateValidator[T]
	update []UpdateValidator[T]
	delete []DeleteValidator[T]

	predicates []Predicate[T]

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

func (v *Validator[T]) WithPredicates(predicates ...Predicate[T]) *Validator[T] {
	v.predicates = append(v.predicates, predicates...)
	return v
}

func (v *Validator[T]) ValidateCreate(ctx context.Context, obj runtime.Object) (w admission.Warnings, err error) {
	if len(v.create) == 0 {
		err = fmt.Errorf("misconfigured webhook rules: create operation not implemented")
		v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", logger.SlogErr(err))
		return nil, nil
	}

	o, err := v.newObject(obj)
	if err != nil {
		return nil, err
	}
	if !v.needValidate(o) {
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

	oldO, err := v.newObject(oldObj)
	if err != nil {
		return nil, err
	}
	newO, err := v.newObject(newObj)
	if err != nil {
		return nil, err
	}
	if !v.needValidate(newO) {
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

	o, err := v.newObject(obj)
	if err != nil {
		return nil, err
	}
	if !v.needValidate(o) {
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

func (v *Validator[T]) needValidate(obj T) bool {
	for _, predicate := range v.predicates {
		if !predicate(obj) {
			return false
		}
	}
	return true
}
