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

package internal

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AttacheeHandler struct {
	client client.Client
}

func NewAttacheeHandler(client client.Client) *AttacheeHandler {
	return &AttacheeHandler{
		client: client,
	}
}

func (h AttacheeHandler) Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("attachee"))

	hasAttachedVM, err := h.hasAttachedVM(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case !hasAttachedVM:
		log.Debug("Allow cluster virtual image deletion")
		controllerutil.RemoveFinalizer(cvi, virtv2.FinalizerCVIProtection)
	case cvi.DeletionTimestamp == nil:
		log.Debug("Protect cluster virtual image from deletion")
		controllerutil.AddFinalizer(cvi, virtv2.FinalizerCVIProtection)
	default:
		log.Debug("Cluster virtual image deletion is delayed: it's protected by virtual machines")
	}

	return reconcile.Result{}, nil
}

func (h AttacheeHandler) hasAttachedVM(ctx context.Context, cvi client.Object) (bool, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		if h.isCVIAttachedToVM(cvi.GetName(), vm) {
			return true, nil
		}
	}

	return false, nil
}

func (h AttacheeHandler) isCVIAttachedToVM(cviName string, vm virtv2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.ClusterImageDevice && bda.Name == cviName {
			return true
		}
	}

	return false
}
