//go:build !linux

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

package netlinkmanager

import (
	"fmt"
	"net"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"
	"vm-route-forge/internal/netlinkwrap"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const DefaultCiliumRouteTable = 1490

type Manager struct{}

func New(
	cache vmipcache.Cache,
	log logr.Logger,
	routeTableID int,
	cidrs []*net.IPNet,
	nlWrapper *netlinkwrap.Funcs,
) *Manager {
	_ = cache
	_ = log
	_ = routeTableID
	_ = cidrs
	_ = nlWrapper
	return &Manager{}
}

func (m *Manager) AddSubnetsRoutesToBlackHole() error {
	return fmt.Errorf("netlink route management is supported only on linux")
}

func (m *Manager) SyncRules() error {
	return fmt.Errorf("netlink rule management is supported only on linux")
}

func (m *Manager) UpdateRoute(_ *v1alpha2.VirtualMachine, _ *ciliumv2.CiliumNode) error {
	return fmt.Errorf("netlink route management is supported only on linux")
}

func (m *Manager) DeleteRoute(_ types.NamespacedName, _ string) error {
	return fmt.Errorf("netlink route management is supported only on linux")
}
