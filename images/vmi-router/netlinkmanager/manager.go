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
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/node/addressing"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"vmi-router/netlinkwrap"
	"vmi-router/netutil"
)

const (
	CiliumIfaceName         = "cilium_host"
	DefaultCiliumRouteTable = 1490
	LocalRouteTable         = 255
)

type Manager struct {
	client    client.Client
	log       logr.Logger
	nlWrapper *netlinkwrap.Funcs
	tableId   int
	cidrs     []*net.IPNet
	nodeName  string
	vmIPs     map[string]string
	vmIPsLock sync.RWMutex
}

func New(client client.Client, log logr.Logger, tableId int, cidrs []*net.IPNet, dryRun bool) *Manager {
	nlWrapper := netlinkwrap.NewFuncs()
	if dryRun {
		nlWrapper = netlinkwrap.DryRunFuncs()
	}
	return &Manager{
		client:    client,
		log:       log,
		tableId:   tableId,
		cidrs:     cidrs,
		nlWrapper: nlWrapper,
		vmIPs:     make(map[string]string),
	}
}

// SyncRules adds rules for configured CIDRS into the Cilium table.
// Also, it removes existing rules for previously configured CIDRs.
func (m *Manager) SyncRules() error {
	// Get rules state.
	rules, err := m.nlWrapper.RuleListFiltered(netlinkwrap.FAMILY_ALL, &netlink.Rule{Table: m.tableId}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("failed to list rules: %v", err)
	}

	// Add rules for configured CIDRs.
	cidrIdx := make(map[string]struct{})
	for _, cidr := range m.cidrs {
		rule := netlink.NewRule()
		rule.Table = m.tableId
		rule.Priority = m.tableId
		rule.Dst = cidr
		cidrIdx[cidr.String()] = struct{}{}
		if err := m.nlWrapper.RuleAdd(rule); err != nil && !os.IsExist(err) {
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
		err := m.nlWrapper.RuleDel(&rule)
		if err != nil {
			m.log.Error(err, fmt.Sprintf("failed to deleted rule %s", rule.String()))
		} else {
			m.log.Info(fmt.Sprintf("deleted %s", rule.String()))
		}
	}

	return nil
}

func (m *Manager) SyncRoutes(ctx context.Context) error {
	// List all Virtual Machines to collect all IPs on this Node.
	vmList := &virtv1alpha2.VirtualMachineList{}
	err := m.client.List(ctx, vmList)
	if err != nil {
		return fmt.Errorf("list VirtualMachines: %w", err)
	}

	vmIPsIdx := make(map[string]struct{})

	// Collect managed IPs from all VirtualMachines in the cluster.
	for _, vm := range vmList.Items {
		vmIP := vm.Status.IPAddress
		if vmIP == "" {
			continue
		}
		isManaged, err := m.isManagedIP(vmIP)
		if err != nil {
			m.log.Error(err, fmt.Sprintf("failed to parse IP address from status in VM %s/%s", vm.GetNamespace(), vm.GetName()))
			continue
		}
		if !isManaged {
			m.log.Info(fmt.Sprintf("Ignore not managed IP %s assigned to VM %s/%s", vmIP, vm.GetNamespace(), vm.GetName()))
		}
		// Save managed IP to index.
		vmIPsIdx[vmIP] = struct{}{}
	}

	// Remove routes unknown for vm IPs.
	nodeRoutes, err := m.nlWrapper.RouteListFiltered(netlinkwrap.FAMILY_ALL, &netlink.Route{Table: m.tableId}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("failed to list node routes: %v", err)
	}

	for _, route := range nodeRoutes {
		// Ignore routes without Dst.
		if route.Dst == nil {
			continue
		}
		// Ignore routes with managed IPs.
		routeIP := route.Dst.IP.String()
		if _, ok := vmIPsIdx[routeIP]; ok {
			continue
		}

		if err := m.nlWrapper.RouteDel(&route); err != nil {
			return fmt.Errorf("failed to delete stale route '%s': %v", fmtRoute(route), err)
		}
		m.log.Info(fmt.Sprintf("route %s removed", fmtRoute(route)))
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
func (m *Manager) UpdateRoute(ctx context.Context, vm *virtv1alpha2.VirtualMachine) {
	// TODO Add cleanup if node was lost?
	// TODO What about migration? Is nodeName just changed to new node or we need some workarounds when 2 Pods are running?
	if vm.Status.Node == "" {
		// VMI has no node assigned
		return
	}
	vmIP := vm.Status.IPAddress
	if vmIP == "" {
		// VMI has no IP address assigned
		return
	}

	isManaged, err := m.isManagedIP(vmIP)
	if err != nil {
		m.log.Error(err, "failed to parse IP address in VM status")
		return
	}
	if !isManaged {
		m.log.Info(fmt.Sprintf("Ignore not managed IP %s assigned to VM/%s", vmIP, vm.GetName()))
		return
	}

	// Prepare ip with the mask to use as the route destination.
	vmIPWithNetmask := netutil.AppendHostNetmask(vmIP)
	_, vmRouteDst, err := net.ParseCIDR(netutil.AppendHostNetmask(vmIP))
	if err != nil {
		m.log.Error(err, fmt.Sprintf("failed to parse IP with netmask %s for vm/%s", vmIPWithNetmask, vm.GetName()))
		return
	}

	// Save IP to the in-memory cache to restore IP later.
	vmiKey := fmt.Sprintf("%s/%s", vm.GetNamespace(), vm.GetName())
	m.vmIPsLock.Lock()
	m.vmIPs[vmiKey] = vmIP
	m.vmIPsLock.Unlock()

	// Retrieve a Cilium Node by VMs node name.
	ciliumNode := &ciliumv2.CiliumNode{}
	err = m.client.Get(ctx, types.NamespacedName{Namespace: "", Name: vm.Status.Node}, ciliumNode)
	if err != nil {
		m.log.Error(err, "failed to get cilium node for vmi")
	}
	nodeIP := getCiliumInternalIPAddress(ciliumNode)
	if nodeIP == "" {
		m.log.Error(nil, "CiliumNode has no %s specified\n", addressing.NodeCiliumInternalIP)
		return
	}
	nodeIPx := net.ParseIP(nodeIP)
	if len(nodeIPx) == 0 {
		m.log.Error(fmt.Errorf(nodeIP), "failed to parse IP address")
		return
	}

	// Get route for specific nodeIP and create similar for our Virtual Machine.
	routes, err := m.nlWrapper.RouteGet(nodeIPx)
	if err != nil || len(routes) == 0 {
		m.log.Error(err, "failed to get route for node")
	}
	origRoute := routes[0]
	route := routes[0]

	// Change iface to cilium if route already exists in local table.
	if route.Table == LocalRouteTable {
		iface, err := netlink.LinkByName(CiliumIfaceName)
		if err != nil {
			m.log.Error(err, fmt.Sprintf("failed to get cilium interface %s", CiliumIfaceName))
			os.Exit(1)
		}
		// Overwrite `lo` interface with `cilium_host`
		route.LinkIndex = iface.Attrs().Index
	}

	route.Dst = vmRouteDst
	route.Table = m.tableId
	route.Type = 1

	if err := m.nlWrapper.RouteReplace(&route); err != nil {
		m.log.Error(err, fmt.Sprintf("failed to update route '%s' to '%s' for VM %s/%s", fmtRoute(origRoute), fmtRoute(route), vm.GetNamespace(), vm.GetName()))
	}
	m.log.Info(fmt.Sprintf("route '%s' updated for VM %s/%s", fmtRoute(route), vm.GetNamespace(), vm.GetName()))
}

func getCiliumInternalIPAddress(node *ciliumv2.CiliumNode) string {
	for _, address := range node.Spec.Addresses {
		if address.Type == addressing.NodeCiliumInternalIP {
			return address.IP
		}
	}
	return ""
}

func (m *Manager) DeleteRoute(vmKey types.NamespacedName, vmIP string) {
	if vmIP == "" {
		// Try to recover IP from the cache.
		m.vmIPsLock.RLock()
		vmIP = m.vmIPs[vmKey.String()]
		m.vmIPsLock.RUnlock()
	}
	if vmIP == "" {
		m.log.Info(fmt.Sprintf("Can't retrieve IP for VM %q, it may lead to stale routes.", vmKey.String()))
		return
	}

	// Prepare ip with the mask to use as the route destination.
	vmIPWithNetmask := netutil.AppendHostNetmask(vmIP)
	_, vmRouteDst, err := net.ParseCIDR(netutil.AppendHostNetmask(vmIP))
	if err != nil {
		m.log.Error(err, fmt.Sprintf("failed to parse IP with netmask %s for VM %q", vmIPWithNetmask, vmKey.String()))
		return
	}

	route := netlink.Route{
		Dst:   vmRouteDst,
		Table: m.tableId,
	}

	if err := m.nlWrapper.RouteDel(&route); err != nil && !os.IsNotExist(err) {
		m.log.Error(err, "failed to delete route")
	}
	m.log.Info(fmt.Sprintf("route %s deleted for VM %q", fmtRoute(route), vmKey))

	// Delete IP from the cache.
	m.vmIPsLock.Lock()
	delete(m.vmIPs, vmKey.String())
	m.vmIPsLock.Unlock()
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
