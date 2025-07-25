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

package vi

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type Validator struct {
	logger    *log.Logger
	client    client.Client
	scService *intsvc.VirtualImageStorageClassService
}

func NewValidator(logger *log.Logger, client client.Client, scService *intsvc.VirtualImageStorageClassService) *Validator {
	return &Validator{
		logger:    logger.With("webhook", "validator"),
		client:    client,
		scService: scService,
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vi, ok := obj.(*virtv2.VirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	}

	if strings.Contains(vi.Name, ".") {
		return nil, fmt.Errorf("the VirtualImage name %q is invalid: '.' is forbidden, allowed name symbols are [0-9a-zA-Z-]", vi.Name)
	}

	if len(vi.Name) > validate.MaxVirtualImageNameLen {
		return nil, fmt.Errorf("the VirtualImage name %q is too long: it must be no more than %d characters", vi.Name, validate.MaxVirtualImageNameLen)
	}

	if vi.Spec.Storage == virtv2.StorageKubernetes {
		warnings := admission.Warnings{
			fmt.Sprintf("Using the `%s` storage type is deprecated. It is recommended to use `%s` instead.",
				virtv2.StorageKubernetes, virtv2.StoragePersistentVolumeClaim),
		}
		return warnings, nil
	}

	if vi.Spec.PersistentVolumeClaim.StorageClass != nil && *vi.Spec.PersistentVolumeClaim.StorageClass != "" {
		sc, err := v.scService.GetStorageClass(ctx, *vi.Spec.PersistentVolumeClaim.StorageClass)
		if err != nil {
			return nil, err
		}
		if v.scService.IsStorageClassDeprecated(sc) {
			return nil, fmt.Errorf(
				"the provisioner of the %q storage class is deprecated; please use a different one",
				*vi.Spec.PersistentVolumeClaim.StorageClass,
			)
		}
	}

	return nil, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVI, ok := oldObj.(*virtv2.VirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualImage but got a %T", newObj)
	}

	newVI, ok := newObj.(*virtv2.VirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualImage but got a %T", newObj)
	}

	v.logger.Info("Validating VirtualImage")

	var warnings admission.Warnings

	if oldVI.Generation == newVI.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vicondition.ReadyType, newVI.Status.Conditions)
	switch {
	case ready.Status == metav1.ConditionTrue, newVI.Status.Phase == virtv2.ImageReady, newVI.Status.Phase == virtv2.ImageLost:
		if !reflect.DeepEqual(oldVI.Spec.DataSource, newVI.Spec.DataSource) {
			return nil, errors.New("data source cannot be changed if the VirtualImage has already been provisioned")
		}

		if !reflect.DeepEqual(oldVI.Spec.PersistentVolumeClaim.StorageClass, newVI.Spec.PersistentVolumeClaim.StorageClass) {
			return nil, errors.New("storage class cannot be changed if the VirtualImage has already been provisioned")
		}
	case newVI.Status.Phase == virtv2.ImageTerminating:
		if !reflect.DeepEqual(oldVI.Spec, newVI.Spec) {
			return nil, errors.New("spec cannot be changed if the VirtualImage is the process of termination")
		}
	case newVI.Status.Phase == virtv2.ImagePending:
		if newVI.Spec.PersistentVolumeClaim.StorageClass != nil && *newVI.Spec.PersistentVolumeClaim.StorageClass != "" {
			sc, err := v.scService.GetStorageClass(ctx, *newVI.Spec.PersistentVolumeClaim.StorageClass)
			if err != nil {
				return nil, err
			}
			if v.scService.IsStorageClassDeprecated(sc) {
				return nil, fmt.Errorf(
					"the provisioner of the %q storage class is deprecated; please use a different one",
					*newVI.Spec.PersistentVolumeClaim.StorageClass,
				)
			}
		}
	}

	if strings.Contains(newVI.Name, ".") {
		warnings = append(warnings, fmt.Sprintf(" the VirtualImage name %q is invalid as it contains now forbidden symbol '.', allowed symbols for name are [0-9a-zA-Z-]. Create another image with valid name to avoid problems with future updates.", newVI.Name))
	}

	if len(newVI.Name) > validate.MaxVirtualImageNameLen {
		warnings = append(warnings, fmt.Sprintf("the VirtualImage name %q is too long: it must be no more than %d characters", newVI.Name, validate.MaxVirtualImageNameLen))
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
