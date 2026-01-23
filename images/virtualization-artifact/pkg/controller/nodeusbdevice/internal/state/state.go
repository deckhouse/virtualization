/*
Copyright 2025 Flant JSC

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

package state

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NodeUSBDeviceState interface {
	NodeUSBDevice() reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus]
	ResourceSlices(ctx context.Context) ([]v1alpha2.ResourceSlice, error)
}

func New(client client.Client, nodeUSBDevice reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus]) NodeUSBDeviceState {
	return &nodeUSBDeviceState{
		client:        client,
		nodeUSBDevice: nodeUSBDevice,
	}
}

type nodeUSBDeviceState struct {
	client        client.Client
	nodeUSBDevice reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus]
}

func (s *nodeUSBDeviceState) NodeUSBDevice() reconciler.Resource[*v1alpha2.NodeUSBDevice, v1alpha2.NodeUSBDeviceStatus] {
	return s.nodeUSBDevice
}

func (s *nodeUSBDeviceState) ResourceSlices(ctx context.Context) ([]v1alpha2.ResourceSlice, error) {
	// TODO: implement ResourceSlice fetching
	// This should fetch ResourceSlice resources that contain USB device information
	return nil, nil
}
