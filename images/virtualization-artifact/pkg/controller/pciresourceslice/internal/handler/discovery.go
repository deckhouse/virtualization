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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/pciresourceslice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameDiscoveryHandler = "DiscoveryHandler"
	draDriverName        = "virtualization-pci"
)

func NewDiscoveryHandler(client client.Client) *DiscoveryHandler {
	return &DiscoveryHandler{client: client}
}

type DiscoveryHandler struct {
	client client.Client
}

func (h *DiscoveryHandler) Name() string {
	return nameDiscoveryHandler
}

func (h *DiscoveryHandler) Handle(ctx context.Context, s state.ResourceSliceState) (reconcile.Result, error) {
	resourceSlice := s.ResourceSlice()
	if resourceSlice == nil {
		return reconcile.Result{}, nil
	}
	if resourceSlice.Spec.Driver != draDriverName {
		return reconcile.Result{}, nil
	}

	for _, device := range resourceSlice.Spec.Devices {
		if !IsPCIDevice(device) {
			continue
		}

		attributes := ConvertDeviceToAttributes(device, resourceSlice.Spec.Pool.Name)
		if err := h.createNodePCIDevice(ctx, attributes); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) createNodePCIDevice(ctx context.Context, attributes v1alpha2.NodePCIDeviceAttributes) error {
	name := h.sanitizeName(attributes.Name)

	existing := &v1alpha2.NodePCIDevice{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name}, existing)
	if err == nil {
		if existing.Status.Attributes != attributes || existing.Status.NodeName != attributes.NodeName {
			existing.Status.Attributes = attributes
			existing.Status.NodeName = attributes.NodeName
			if err := h.client.Status().Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to update NodePCIDevice status: %w", err)
			}
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check if NodePCIDevice exists: %w", err)
	}

	nodePCIDevice := &v1alpha2.NodePCIDevice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1alpha2.NodePCIDeviceSpec{AssignedNamespace: ""},
	}

	if err := h.client.Create(ctx, nodePCIDevice); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create NodePCIDevice: %w", err)
	}

	nodePCIDevice.Status = v1alpha2.NodePCIDeviceStatus{
		Attributes: attributes,
		NodeName:   attributes.NodeName,
	}

	if err := h.client.Status().Update(ctx, nodePCIDevice); err != nil {
		return fmt.Errorf("failed to update NodePCIDevice status: %w", err)
	}

	return nil
}

func (h *DiscoveryHandler) sanitizeName(deviceName string) string {
	return strings.ToLower(strings.ReplaceAll(deviceName, ".", "-"))
}
