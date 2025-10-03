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
	"errors"
	"fmt"
	"net"
	"os"
	"vm-route-forge/internal/netlinkwrap"
	"vm-route-forge/internal/netutil"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/node/addressing"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	CiliumIfaceName         = "cilium_host"
	DefaultCiliumRouteTable = 1490
	LocalRouteTable         = 255
	netlinkManager          = "netlinkManager"
	routePriority           = 0
	blackHoleRoutePriority  = 100
)

type Manager struct {
	log          logr.Logger
	nlWrapper    *netlinkwrap.Funcs
	routeTableID int
	cidrs        []*net.IPNet
	nodeName     string
	cache        vmipcache.Cache
}

func New(cache vmipcache.Cache,
	log logr.Logger,
	routeTableID int,
	cidrs []*net.IPNet,
	nlWrapper *netlinkwrap.Funcs,
) *Manager {
	return &Manager{
		log:          log.WithValues("manager", netlinkManager),
		routeTableID: routeTableID,
		cidrs:        cidrs,
		nlWrapper:    nlWrapper,
		cache:        cache,
	}
}

func (m *Manager) AddSubnetsRoutesToBlackHole() error {
	for _, cidr := range m.cidrs {
		route := &netlink.Route{
			Scope:    netlink.SCOPE_UNIVERSE,
			Dst:      cidr,
			Table:    m.routeTableID,
			Type:     unix.RTN_BLACKHOLE,
			Priority: blackHoleRoutePriority,
		}
		if err := m.nlWrapper.RouteReplace(route); err != nil {
			return fmt.Errorf("failed to update route: %w", err)
		}
	}

	return nil
}

// SyncRules adds rules for configured CIDRS into the Cilium table.
// Also, it removes existing rules for previously configured CIDRs.
func (m *Manager) SyncRules() error {
	// Get rules state.
	rules, err := m.nlWrapper.RuleListFiltered(netlink.FAMILY_ALL, &netlink.Rule{Table: m.routeTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("failed to list rules: %v", err)
	}

	// Add rules for configured CIDRs.
	cidrIdx := make(map[string]struct{})
	for _, cidr := range m.cidrs {
		rule := netlink.NewRule()
		rule.Table = m.routeTableID
		rule.Priority = m.routeTableID
		rule.Dst = cidr
		cidrIdx[cidr.String()] = struct{}{}
		if err = m.nlWrapper.RuleAdd(rule); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to add rule: %v", err)
		}
		m.log.Info(fmt.Sprintf("rule %s added", rule.String()))
	}

	// Remove rules for previously configured CIDRs.
	for _, rule := range rules {
		// Ignore rules without Dst.
		if rule.Dst == nil {
			continue
		}
		// Ignore rules for configured CIDRs.
		cidr := rule.Dst.String()
		if _, ok := cidrIdx[cidr]; ok {
			continue
		}

		// Dst is not for the configured CIDR, remove it.
		err = m.nlWrapper.RuleDel(&rule)
		if err != nil {
			return fmt.Errorf("failed to deleted rule %s: %w", rule.String(), err)
		} else {
			m.log.Info(fmt.Sprintf("deleted %s", rule.String()))
		}
	}

	return nil
}

// isManagedIP detects if IP belongs to configured CIDRs.
func (m *Manager) isManagedIP(ip string) (bool, error) {
	netIP := net.ParseIP(ip)
	if len(netIP) == 0 {
		return false, fmt.Errorf("invalid IP address %s", ip)
	}

	for _, cidr := range m.cidrs {
		if cidr.Contains(netIP) {
			return true, nil
		}
	}

	return false, nil
}

// UpdateRoute updates route for a single VirtualMachine.
func (m *Manager) UpdateRoute(vm *v1alpha2.VirtualMachine, ciliumNode *ciliumv2.CiliumNode) error {
	// TODO Add cleanup if node was lost?
	// TODO What about migration? Is nodeName just changed to new node or we need some workarounds when 2 Pods are running?
	if vm == nil {
		return nil
	}
	nodeIP := getCiliumInternalIPAddress(ciliumNode)
	if nodeIP == "" {
		return fmt.Errorf("ciliumNode has no %s specified", addressing.NodeCiliumInternalIP)
	}
	nodeIPx := net.ParseIP(nodeIP)
	if len(nodeIPx) == 0 {
		return fmt.Errorf("invalid IP address %s", nodeIP)
	}

	vmIP := vm.Status.IPAddress
	if vmIP == "" {
		// VM has no IP address assigned
		return nil
	}

	isManaged, err := m.isManagedIP(vmIP)
	if err != nil {
		return fmt.Errorf("failed to parse IP address in VM status: %w", err)
	}
	if !isManaged {
		m.log.Info(fmt.Sprintf("Ignore not managed IP %s assigned to VM/%s", vmIP, vm.GetName()))
		return nil
	}

	// Prepare ip with the mask to use as the route destination.
	vmIPWithNetmask := netutil.AppendHostNetmask(vmIP)
	_, vmRouteDst, err := net.ParseCIDR(netutil.AppendHostNetmask(vmIP))
	if err != nil {
		return fmt.Errorf("failed to parse IP with netmask %s for vm/%s: %w", vmIPWithNetmask, vm.GetName(), err)
	}

	// Save IP to the in-memory cache to restore IP later.
	vmKey := types.NamespacedName{Name: vm.GetName(), Namespace: vm.GetNamespace()}
	m.cache.Set(vmKey, vmipcache.Addresses{VMIP: vmipcache.IP(vmIP), NodeIP: vmipcache.IP(nodeIP)})

	// Get route for specific nodeIP and create similar for our Virtual Machine.
	routes, err := m.nlWrapper.RouteGet(nodeIPx)
	if err != nil || len(routes) == 0 {
		return fmt.Errorf("failed to get routes for node %s: %w", nodeIPx, err)
	}
	origRoute := routes[0]
	route := routes[0]

	// Change iface to cilium if route already exists in local table.
	if route.Table == LocalRouteTable {
		iface, err := netlink.LinkByName(CiliumIfaceName)
		if err != nil {
			return fmt.Errorf("failed to get cilium interface %s: %w", CiliumIfaceName, err)
		}
		// Overwrite `lo` interface with `cilium_host`
		route.LinkIndex = iface.Attrs().Index
	}

	route.Dst = vmRouteDst
	route.Table = m.routeTableID
	route.Type = 1
	route.Priority = routePriority

	if err = m.nlWrapper.RouteReplace(&route); err != nil {
		m.log.Error(err, fmt.Sprintf("failed to update route %q to %q for VM %s/%s", fmtRoute(origRoute), fmtRoute(route), vm.GetNamespace(), vm.GetName()))
		return fmt.Errorf("failed to update route: %w", err)
	}
	m.log.Info(fmt.Sprintf("route %q updated for VM %s/%s", fmtRoute(route), vm.GetNamespace(), vm.GetName()))
	return nil
}

func getCiliumInternalIPAddress(node *ciliumv2.CiliumNode) string {
	if node == nil {
		return ""
	}
	for _, address := range node.Spec.Addresses {
		if address.Type == addressing.NodeCiliumInternalIP {
			return address.IP
		}
	}
	return ""
}

func (m *Manager) DeleteRoute(vmKey types.NamespacedName, vmIP string) error {
	if vmIP == "" {
		// Try to recover IP from the cache.
		if addr, found := m.cache.GetAddresses(vmKey); found {
			vmIP = addr.VMIP.String()
		}
	}
	if vmIP == "" {
		m.log.Info(fmt.Sprintf("Can't retrieve IP for VM %q, it may lead to stale routes.", vmKey.String()))
		return nil
	}

	// Prepare ip with the mask to use as the route destination.
	vmIPWithNetmask := netutil.AppendHostNetmask(vmIP)
	_, vmRouteDst, err := net.ParseCIDR(vmIPWithNetmask)
	if err != nil {
		return fmt.Errorf("failed to parse IP with netmask %s for vm/%s: %w", vmIPWithNetmask, vmKey.String(), err)
	}

	route := netlink.Route{
		Dst:   vmRouteDst,
		Table: m.routeTableID,
	}

	if err := m.nlWrapper.RouteDel(&route); err != nil && !os.IsNotExist(err) && !errors.Is(err, unix.ESRCH) {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	m.log.Info(fmt.Sprintf("route %s deleted for VM %q", fmtRoute(route), vmKey))

	// Delete IP from the cache.
	m.cache.DeleteByKey(vmKey)
	return nil
}

func fmtRoute(route netlink.Route) string {
	dst := ""
	if route.Dst != nil {
		dst = fmt.Sprintf("dst %s", route.Dst.String())
	}
	via := ""
	if route.Via != nil {
		via = fmt.Sprintf("via %s", route.Via.String())
	}
	src := fmt.Sprintf("src %s", route.Src.String())

	return fmt.Sprintf("%s %s %s", dst, via, src)
}
