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

package state

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NodePCIDeviceState interface {
	NodePCIDevice() *reconciler.Resource[*v1alpha2.NodePCIDevice, v1alpha2.NodePCIDeviceStatus]
}

func New(_ client.Client, nodePCIDevice *reconciler.Resource[*v1alpha2.NodePCIDevice, v1alpha2.NodePCIDeviceStatus]) NodePCIDeviceState {
	return &nodePCIDeviceState{nodePCIDevice: nodePCIDevice}
}

type nodePCIDeviceState struct {
	nodePCIDevice *reconciler.Resource[*v1alpha2.NodePCIDevice, v1alpha2.NodePCIDeviceStatus]
}

func (s *nodePCIDeviceState) NodePCIDevice() *reconciler.Resource[*v1alpha2.NodePCIDevice, v1alpha2.NodePCIDeviceStatus] {
	return s.nodePCIDevice
}
