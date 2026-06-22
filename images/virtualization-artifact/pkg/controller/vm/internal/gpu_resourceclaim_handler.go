/*
Copyright 2026 Flant JSC

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

package internal

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameGPUResourceClaimHandler = "GPUResourceClaimHandler"
	gpuDeviceClassName          = "gpu.deckhouse.io"
)

func NewGPUResourceClaimHandler(client client.Client) *GPUResourceClaimHandler {
	return &GPUResourceClaimHandler{client: client}
}

type GPUResourceClaimHandler struct {
	client client.Client
}

func (h *GPUResourceClaimHandler) Name() string {
	return nameGPUResourceClaimHandler
}

func (h *GPUResourceClaimHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Current()
	gpuID := vm.Annotations[annotations.AnnVMGPUID]
	templateName := kvbuilder.GPUResourceClaimTemplateName(vm.Name)
	template := &resourcev1.ResourceClaimTemplate{}
	key := types.NamespacedName{Name: templateName, Namespace: vm.Namespace}

	if gpuID == "" {
		if err := h.client.Get(ctx, key, template); err != nil {
			if apierrors.IsNotFound(err) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, fmt.Errorf("failed to get GPU ResourceClaimTemplate: %w", err)
		}
		if metav1.IsControlledBy(template, vm) {
			if err := h.client.Delete(ctx, template); err != nil && !apierrors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("failed to delete GPU ResourceClaimTemplate: %w", err)
			}
		}
		return reconcile.Result{}, nil
	}

	desiredSpec := buildGPUResourceClaimTemplateSpec(gpuID)
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameGPUResourceClaimHandler))
	err := h.client.Get(ctx, key, template)
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("failed to get GPU ResourceClaimTemplate: %w", err)
	}

	if apierrors.IsNotFound(err) {
		template = buildGPUResourceClaimTemplate(vm, templateName, desiredSpec)
		if err := h.client.Create(ctx, template); err != nil && !apierrors.IsAlreadyExists(err) {
			return reconcile.Result{}, fmt.Errorf("failed to create GPU ResourceClaimTemplate: %w", err)
		}
		log.Info("created GPU ResourceClaimTemplate", "template", templateName)
		return reconcile.Result{}, nil
	}

	if reflect.DeepEqual(template.Spec, desiredSpec) {
		return reconcile.Result{}, nil
	}
	if err := h.client.Delete(ctx, template); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("failed to delete outdated GPU ResourceClaimTemplate: %w", err)
	}
	template = buildGPUResourceClaimTemplate(vm, templateName, desiredSpec)
	if err := h.client.Create(ctx, template); err != nil && !apierrors.IsAlreadyExists(err) {
		return reconcile.Result{}, fmt.Errorf("failed to recreate GPU ResourceClaimTemplate: %w", err)
	}
	log.Info("recreated GPU ResourceClaimTemplate", "template", templateName)
	return reconcile.Result{}, nil
}

func buildGPUResourceClaimTemplate(vm *v1alpha2.VirtualMachine, name string, spec resourcev1.ResourceClaimTemplateSpec) *resourcev1.ResourceClaimTemplate {
	return &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{service.MakeControllerOwnerReference(vm)},
		},
		Spec: spec,
	}
}

func buildGPUResourceClaimTemplateSpec(gpuID string) resourcev1.ResourceClaimTemplateSpec {
	selector := fmt.Sprintf(
		`device.attributes["gpu.deckhouse.io"].gpuUUID == %s && device.attributes["gpu.deckhouse.io"].deviceType == "physical" && !has(device.attributes["gpu.deckhouse.io"].sharingStrategy)`,
		strconv.Quote(gpuID),
	)
	return resourcev1.ResourceClaimTemplateSpec{
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: kvbuilder.GPUResourceClaimRequestName,
					Exactly: &resourcev1.ExactDeviceRequest{
						DeviceClassName: gpuDeviceClassName,
						AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
						Count:           1,
						Selectors: []resourcev1.DeviceSelector{{
							CEL: &resourcev1.CELDeviceSelector{Expression: selector},
						}},
					},
				}},
			},
		},
	}
}
