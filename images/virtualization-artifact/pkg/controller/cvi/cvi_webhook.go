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
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/cvmi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type Validator struct {
	logger *slog.Logger
}

func NewValidator() *Validator {
	return &Validator{
		logger: slog.Default().With("controller", cvmi.ShortName, "webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: create operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldCVI, ok := oldObj.(*virtv2.ClusterVirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected an old ClusterVirtualImage but got a %T", newObj)
	}

	newCVI, ok := newObj.(*virtv2.ClusterVirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new ClusterVirtualImage but got a %T", newObj)
	}

	v.logger.Info("Validating ClusterVirtualImage")

	if oldCVI.Generation == newCVI.Generation {
		return nil, nil
	}

	ready, _ := service.GetCondition(cvicondition.ReadyType, newCVI.Status.Conditions)
	if newCVI.Status.Phase == virtv2.ImageReady || ready.Status == metav1.ConditionTrue {
		return nil, fmt.Errorf("ClusterVirtualImage is in a Ready state: configuration changes are not available")
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
