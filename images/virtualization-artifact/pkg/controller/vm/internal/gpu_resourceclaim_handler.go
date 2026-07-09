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
	"encoding/json"
	"fmt"
	"reflect"

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

const nameGPUResourceClaimHandler = "GPUResourceClaimHandler"

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
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameGPUResourceClaimHandler))
	desiredTemplateNames := make(map[string]struct{}, len(vm.Spec.GPUDevices))

	for _, device := range vm.Spec.GPUDevices {
		templateName := kvbuilder.GPUResourceClaimTemplateName(vm.Name, device.Name)
		desiredTemplateNames[templateName] = struct{}{}
		desiredSpec := buildGPUResourceClaimTemplateSpec(device)
		template := &resourcev1.ResourceClaimTemplate{}
		key := types.NamespacedName{Name: templateName, Namespace: vm.Namespace}

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
			continue
		}

		if !metav1.IsControlledBy(template, vm) {
			return reconcile.Result{}, fmt.Errorf("GPU ResourceClaimTemplate %s/%s is not controlled by VirtualMachine %s/%s", template.Namespace, template.Name, vm.Namespace, vm.Name)
		}

		if gpuClaimTemplateUpToDate(template, desiredSpec) {
			continue
		}
		if err := h.client.Delete(ctx, template); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to delete outdated GPU ResourceClaimTemplate: %w", err)
		}
		template = buildGPUResourceClaimTemplate(vm, templateName, desiredSpec)
		if err := h.client.Create(ctx, template); err != nil && !apierrors.IsAlreadyExists(err) {
			return reconcile.Result{}, fmt.Errorf("failed to recreate GPU ResourceClaimTemplate: %w", err)
		}
		log.Info("recreated GPU ResourceClaimTemplate", "template", templateName)
	}

	if err := h.deleteOrphanedTemplates(ctx, vm, desiredTemplateNames); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func buildGPUResourceClaimTemplate(vm *v1alpha2.VirtualMachine, name string, spec resourcev1.ResourceClaimTemplateSpec) *resourcev1.ResourceClaimTemplate {
	return &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       vm.Namespace,
			Annotations:     map[string]string{annotations.AnnGPUClaimSpecHash: gpuClaimSpecHash(spec)},
			OwnerReferences: []metav1.OwnerReference{service.MakeControllerOwnerReference(vm)},
		},
		Spec: spec,
	}
}

// gpuClaimTemplateUpToDate prefers the spec-hash annotation over comparing the
// stored spec directly: API-server defaulting could make the stored spec
// permanently differ from the rendered one and loop delete/recreate.
// Templates created before the annotation existed fall back to DeepEqual and
// migrate to the hash on their next legitimate recreation.
func gpuClaimTemplateUpToDate(template *resourcev1.ResourceClaimTemplate, desiredSpec resourcev1.ResourceClaimTemplateSpec) bool {
	storedHash, ok := template.Annotations[annotations.AnnGPUClaimSpecHash]
	if !ok {
		return reflect.DeepEqual(template.Spec, desiredSpec)
	}
	return storedHash == gpuClaimSpecHash(desiredSpec)
}

func gpuClaimSpecHash(spec resourcev1.ResourceClaimTemplateSpec) string {
	// Marshalling a plain API struct cannot fail; on the impossible failure both
	// sides hash the same empty payload, so the comparison still converges.
	raw, _ := json.Marshal(&spec)
	return kvbuilder.GenerateSerial(string(raw))
}

func buildGPUResourceClaimTemplateSpec(device v1alpha2.GPUDeviceSpec) resourcev1.ResourceClaimTemplateSpec {
	requestName := kvbuilder.GPUResourceClaimName(device.Name)
	return resourcev1.ResourceClaimTemplateSpec{
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: requestName,
					Exactly: &resourcev1.ExactDeviceRequest{
						DeviceClassName: device.DeviceClassName,
						AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
						Count:           1,
					},
				}},
				Config: []resourcev1.DeviceClaimConfiguration{{
					Requests: []string{requestName},
					DeviceConfiguration: resourcev1.DeviceConfiguration{
						Opaque: &resourcev1.OpaqueDeviceConfiguration{
							Driver:     kvbuilder.GPUDRADriverName,
							Parameters: runtime.RawExtension{Raw: []byte(`{"apiVersion":"resource.gpu.deckhouse.io/v1alpha1","kind":"VfioDeviceConfig"}`)},
						},
					},
				}},
			},
		},
	}
}

func (h *GPUResourceClaimHandler) deleteOrphanedTemplates(ctx context.Context, vm *v1alpha2.VirtualMachine, desiredTemplateNames map[string]struct{}) error {
	var templates resourcev1.ResourceClaimTemplateList
	if err := h.client.List(ctx, &templates, client.InNamespace(vm.Namespace)); err != nil {
		return fmt.Errorf("failed to list GPU ResourceClaimTemplates: %w", err)
	}

	for i := range templates.Items {
		template := &templates.Items[i]
		if !metav1.IsControlledBy(template, vm) || !kvbuilder.IsGPUResourceClaimTemplateName(vm.Name, template.Name) {
			continue
		}
		if _, ok := desiredTemplateNames[template.Name]; ok {
			continue
		}
		if err := h.client.Delete(ctx, template); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete GPU ResourceClaimTemplate: %w", err)
		}
	}
	return nil
}
