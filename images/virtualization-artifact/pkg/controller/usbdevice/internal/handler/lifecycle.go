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

package handler

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

const (
	nameLifecycleHandler            = "LifecycleHandler"
	resourceClaimTemplateNameSuffix = "-template"
)

func ResourceClaimTemplateName(usbDeviceName string) string {
	return usbDeviceName + resourceClaimTemplateNameSuffix
}

func NewLifecycleHandler(client client.Client) *LifecycleHandler {
	return &LifecycleHandler{
		client: client,
	}
}

type LifecycleHandler struct {
	client client.Client
}

func (h *LifecycleHandler) Name() string {
	return nameLifecycleHandler
}

func (h *LifecycleHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	if s.USBDevice().IsEmpty() {
		return reconcile.Result{}, nil
	}

	if err := h.syncReady(ctx, s); err != nil {
		return reconcile.Result{}, err
	}

	if err := h.ensureResourceClaimTemplate(ctx, s); err != nil {
		return reconcile.Result{}, err
	}

	if err := h.syncAttached(ctx, s); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (h *LifecycleHandler) syncReady(ctx context.Context, s state.USBDeviceState) error {
	current := s.USBDevice().Current()
	changed := s.USBDevice().Changed()

	nodeUSBDevice, err := s.NodeUSBDevice(ctx)
	if err != nil {
		return err
	}

	if nodeUSBDevice == nil {
		setReadyCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, usbdevicecondition.NotFound, "Corresponding NodeUSBDevice not found.", nil)
		return nil
	}

	if !equality.Semantic.DeepEqual(changed.Status.Attributes, nodeUSBDevice.Status.Attributes) || changed.Status.NodeName != nodeUSBDevice.Status.NodeName {
		changed.Status.Attributes = nodeUSBDevice.Status.Attributes
		changed.Status.NodeName = nodeUSBDevice.Status.NodeName
	}

	readyCondition := meta.FindStatusCondition(nodeUSBDevice.Status.Conditions, string(nodeusbdevicecondition.ReadyType))
	if readyCondition == nil {
		setReadyCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, usbdevicecondition.NotReady, "Ready condition not found in NodeUSBDevice.", nil)
		return nil
	}

	var reason usbdevicecondition.ReadyReason
	var status metav1.ConditionStatus

	switch readyCondition.Reason {
	case string(nodeusbdevicecondition.Ready):
		reason = usbdevicecondition.Ready
		status = metav1.ConditionTrue
	case string(nodeusbdevicecondition.NotReady):
		reason = usbdevicecondition.NotReady
		status = metav1.ConditionFalse
	case string(nodeusbdevicecondition.NotFound):
		reason = usbdevicecondition.NotFound
		status = metav1.ConditionFalse
	default:
		reason = usbdevicecondition.NotReady
		status = metav1.ConditionFalse
	}

	setReadyCondition(current, &changed.Status.Conditions, status, reason, readyCondition.Message, &readyCondition.LastTransitionTime)

	return nil
}

func (h *LifecycleHandler) ensureResourceClaimTemplate(ctx context.Context, s state.USBDeviceState) error {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameLifecycleHandler))
	usbDevice := s.USBDevice().Current()

	if usbDevice.Status.Attributes.Name == "" {
		log.Debug("USBDevice has no attributes name yet, skipping ResourceClaimTemplate")
		return nil
	}

	templateName := ResourceClaimTemplateName(usbDevice.Name)
	template := &resourcev1.ResourceClaimTemplate{}
	key := types.NamespacedName{Name: templateName, Namespace: usbDevice.Namespace}
	desiredSpec := buildResourceClaimTemplateSpec(usbDevice)

	err := h.client.Get(ctx, key, template)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get ResourceClaimTemplate: %w", err)
	}

	if !apierrors.IsNotFound(err) {
		if !reflect.DeepEqual(template.Spec, desiredSpec) {
			if err := h.client.Delete(ctx, template); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete outdated ResourceClaimTemplate: %w", err)
			}

			template = &resourcev1.ResourceClaimTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:            templateName,
					Namespace:       usbDevice.Namespace,
					OwnerReferences: []metav1.OwnerReference{service.MakeControllerOwnerReference(usbDevice)},
				},
				Spec: desiredSpec,
			}

			if err := h.client.Create(ctx, template); err != nil {
				return fmt.Errorf("failed to recreate ResourceClaimTemplate: %w", err)
			}

			log.Info("recreated ResourceClaimTemplate for USBDevice", "template", templateName)
		}
		return nil
	}

	template = &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            templateName,
			Namespace:       usbDevice.Namespace,
			OwnerReferences: []metav1.OwnerReference{service.MakeControllerOwnerReference(usbDevice)},
		},
		Spec: desiredSpec,
	}

	if err := h.client.Create(ctx, template); err != nil {
		return fmt.Errorf("failed to create ResourceClaimTemplate: %w", err)
	}

	log.Info("created ResourceClaimTemplate for USBDevice", "template", templateName)
	return nil
}

func (h *LifecycleHandler) syncAttached(ctx context.Context, s state.USBDeviceState) error {
	current := s.USBDevice().Current()
	changed := s.USBDevice().Changed()

	vms, err := s.VirtualMachinesUsingDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to find VirtualMachines using USBDevice: %w", err)
	}

	var reason usbdevicecondition.AttachedReason
	var status metav1.ConditionStatus
	var message string

	if len(vms) == 0 {
		reason = usbdevicecondition.Available
		status = metav1.ConditionFalse
		message = "Device is available for attachment to a virtual machine."
		setAttachedCondition(current, &changed.Status.Conditions, status, reason, message)
		return nil
	}

	reason = usbdevicecondition.AttachedToVirtualMachine
	status = metav1.ConditionTrue
	message = fmt.Sprintf("Device is attached to %d VirtualMachines.", len(vms))
	if len(vms) == 1 {
		message = fmt.Sprintf("Device is attached to VirtualMachine %s/%s.", vms[0].Namespace, vms[0].Name)
	}

	setAttachedCondition(current, &changed.Status.Conditions, status, reason, message)
	return nil
}

func buildResourceClaimTemplateSpec(usbDevice *v1alpha2.USBDevice) resourcev1.ResourceClaimTemplateSpec {
	attributes := usbDevice.Status.Attributes
	selectorDeviceName := attributes.Name
	if selectorDeviceName == "" {
		selectorDeviceName = usbDevice.Name
	}

	return resourcev1.ResourceClaimTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.AnnUSBDeviceGroup: annotations.DefaultUSBDeviceGroup,
				annotations.AnnUSBDeviceUser:  annotations.DefaultUSBDeviceUser,
			},
		},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: "req-" + usbDevice.Name,
					Exactly: &resourcev1.ExactDeviceRequest{
						Count:           1,
						AllocationMode:  resourcev1.DeviceAllocationModeExactCount,
						DeviceClassName: "usb-devices.virtualization.deckhouse.io",
						Selectors: []resourcev1.DeviceSelector{{
							CEL: &resourcev1.CELDeviceSelector{Expression: fmt.Sprintf(`device.attributes["virtualization-usb"].name == %s`, strconv.Quote(selectorDeviceName))},
						}},
					},
				}},
			},
		},
	}
}
