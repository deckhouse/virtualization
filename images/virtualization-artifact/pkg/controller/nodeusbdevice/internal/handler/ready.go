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

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameReadyHandler = "ReadyHandler"
	draDriverName    = "virtualization-usb"
)

func NewReadyHandler(client client.Client) *ReadyHandler {
	return &ReadyHandler{client: client}
}

type ReadyHandler struct {
	client client.Client
}

func (h *ReadyHandler) Name() string {
	return nameReadyHandler
}

func (h *ReadyHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()
	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	readyReason := nodeusbdevicecondition.NotFound
	readyStatus := metav1.ConditionFalse
	readyMessage := "Device is absent on the host."

	found, err := h.deviceExistsOnHost(ctx, current)
	if err != nil {
		return reconcile.Result{}, err
	}
	if found {
		readyReason = nodeusbdevicecondition.Ready
		readyStatus = metav1.ConditionTrue
		readyMessage = "Device is ready to use."
	}

	cb := conditions.NewConditionBuilder(nodeusbdevicecondition.ReadyType).
		Generation(current.GetGeneration()).
		Status(readyStatus).
		Reason(readyReason).
		Message(readyMessage)

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *ReadyHandler) deviceExistsOnHost(ctx context.Context, nodeUSBDevice *v1alpha2.NodeUSBDevice) (bool, error) {
	nodeName := nodeUSBDevice.Status.NodeName
	if nodeName == "" {
		return false, nil
	}

	var slices resourcev1.ResourceSliceList
	if err := h.client.List(ctx, &slices, client.MatchingFields{
		indexer.IndexFieldResourceSliceByPoolName: nodeName,
		indexer.IndexFieldResourceSliceByDriver:   draDriverName,
	}); err != nil {
		return false, fmt.Errorf("failed to list ResourceSlices: %w", err)
	}

	deviceName := nodeUSBDevice.Status.Attributes.Name
	if deviceName == "" {
		deviceName = nodeUSBDevice.GetName()
	}

	for _, slice := range slices.Items {
		for _, dev := range slice.Spec.Devices {
			if dev.Name == deviceName {
				return true, nil
			}
		}
	}

	return false, nil
}
