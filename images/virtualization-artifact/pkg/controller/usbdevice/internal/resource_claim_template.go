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

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameResourceClaimTemplateHandler = "ResourceClaimTemplateHandler"
	resourceClaimTemplateNameSuffix  = "-template"
)

// ResourceClaimTemplateName returns the name of ResourceClaimTemplate for a USBDevice.
func ResourceClaimTemplateName(usbDeviceName string) string {
	return usbDeviceName + resourceClaimTemplateNameSuffix
}

func NewResourceClaimTemplateHandler(client client.Client, scheme *runtime.Scheme) *ResourceClaimTemplateHandler {
	return &ResourceClaimTemplateHandler{
		client: client,
		scheme: scheme,
	}
}

type ResourceClaimTemplateHandler struct {
	client client.Client
	scheme *runtime.Scheme
}

func (h *ResourceClaimTemplateHandler) Name() string {
	return nameResourceClaimTemplateHandler
}

func (h *ResourceClaimTemplateHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameResourceClaimTemplateHandler))

	if s.USBDevice().IsEmpty() {
		return reconcile.Result{}, nil
	}

	usbDevice := s.USBDevice().Current()

	if usbDevice.Status.Attributes.Name == "" {
		log.Debug("USBDevice has no attributes name yet, skipping ResourceClaimTemplate")
		return reconcile.Result{}, nil
	}

	templateName := ResourceClaimTemplateName(usbDevice.Name)
	template := &resourcev1beta1.ResourceClaimTemplate{}
	key := types.NamespacedName{
		Name:      templateName,
		Namespace: usbDevice.Namespace,
	}

	err := h.client.Get(ctx, key, template)
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("failed to get ResourceClaimTemplate: %w", err)
	}

	if apierrors.IsNotFound(err) {
		desiredSpec := h.buildTemplateSpec(usbDevice)
		template = &resourcev1beta1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      templateName,
				Namespace: usbDevice.Namespace,
			},
			Spec: desiredSpec,
		}

		if err := controllerutil.SetControllerReference(usbDevice, template, h.scheme); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to set owner reference on ResourceClaimTemplate: %w", err)
		}

		if err := h.client.Create(ctx, template); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to create ResourceClaimTemplate: %w", err)
		}

		log.Info("created ResourceClaimTemplate for USBDevice", "template", templateName)
	}

	return reconcile.Result{}, nil
}

func (h *ResourceClaimTemplateHandler) buildTemplateSpec(usbDevice *v1alpha2.USBDevice) resourcev1beta1.ResourceClaimTemplateSpec {
	attributes := usbDevice.Status.Attributes
	deviceName := attributes.Name
	if deviceName == "" {
		deviceName = usbDevice.Name
	}

	return resourcev1beta1.ResourceClaimTemplateSpec{
		Spec: resourcev1beta1.ResourceClaimSpec{
			Devices: resourcev1beta1.DeviceClaim{
				Requests: []resourcev1beta1.DeviceRequest{
					{
						Name:            "req-" + deviceName,
						AllocationMode:  resourcev1beta1.DeviceAllocationModeExactCount,
						Count:           1,
						DeviceClassName: "usb-devices.virtualization.deckhouse.io",
						Selectors: []resourcev1beta1.DeviceSelector{
							{
								CEL: &resourcev1beta1.CELDeviceSelector{
									Expression: fmt.Sprintf(`device.attributes["virtualization-dra"].name == "%s"`, deviceName),
								},
							},
						},
					},
				},
			},
		},
	}
}
