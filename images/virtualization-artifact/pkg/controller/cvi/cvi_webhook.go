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

package cvi

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type Validator struct {
	logger *log.Logger
}

func NewValidator(logger *log.Logger) *Validator {
	return &Validator{
		logger: logger.With("webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cvi, ok := obj.(*v1alpha2.ClusterVirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new ClusterVirtualImage but got a %T", obj)
	}

	if strings.Contains(cvi.Name, ".") {
		return nil, fmt.Errorf("the ClusterVirtualImage name %q is invalid: '.' is forbidden, allowed name symbols are [0-9a-zA-Z-]", cvi.Name)
	}

	if len(cvi.Name) > validate.MaxClusterVirtualImageNameLen {
		return nil, fmt.Errorf("the ClusterVirtualImage name %q is too long: it must be no more than %d characters", cvi.Name, validate.MaxClusterVirtualImageNameLen)
	}

	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldCVI, ok := oldObj.(*v1alpha2.ClusterVirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected an old ClusterVirtualImage but got a %T", newObj)
	}

	newCVI, ok := newObj.(*v1alpha2.ClusterVirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new ClusterVirtualImage but got a %T", newObj)
	}

	v.logger.Info("Validating ClusterVirtualImage")

	var warnings admission.Warnings

	if oldCVI.Generation == newCVI.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(cvicondition.ReadyType, newCVI.Status.Conditions)
	if newCVI.Status.Phase == v1alpha2.ImageReady || ready.Status == metav1.ConditionTrue {
		return nil, fmt.Errorf("ClusterVirtualImage is in a Ready state: configuration changes are not available")
	}

	if strings.Contains(newCVI.Name, ".") {
		warnings = append(warnings, fmt.Sprintf("the ClusterVirtualImage name %q is invalid as it contains now forbidden symbol '.', allowed symbols for name are [0-9a-zA-Z-]. Create another image with valid name to avoid problems with future updates.", newCVI.Name))
	}

	if len(newCVI.Name) > validate.MaxClusterVirtualImageNameLen {
		warnings = append(warnings, fmt.Sprintf("the ClusterVirtualImage name %q is too long: it must be no more than %d characters", newCVI.Name, validate.MaxClusterVirtualImageNameLen))
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
