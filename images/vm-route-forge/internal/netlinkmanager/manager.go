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
	"errors"
	"fmt"
	"net"
	"os"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	ciliumv2Informers "github.com/cilium/cilium/pkg/k8s/client/informers/externalversions/cilium.io/v2"
	"github.com/cilium/cilium/pkg/node/addressing"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	virtinformers "github.com/deckhouse/virtualization/api/client/generated/informers/externalversions/core/v1alpha2"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	cache2 "vm-route-forge/internal/cache"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	netlinkwrap2 "vm-route-forge/internal/netlinkwrap"
	"vm-route-forge/internal/netutil"
)

const (
	CiliumIfaceName         = "cilium_host"
	DefaultCiliumRouteTable = 1490
	LocalRouteTable         = 255
	netlinkManager          = "netlinkManager"
)

type Manager struct {
	vmLister  virtlisters.VirtualMachineLister
	cnIndexer cache.Indexer
	hasSynced cache.InformerSynced
	log       logr.Logger
	nlWrapper *netlinkwrap2.Funcs
	tableId   int
	cidrs     []*net.IPNet
	nodeName  string
	cache     cache2.Cache
}

func New(vmInformer virtinformers.VirtualMachineInformer,
	cnInformer ciliumv2Informers.CiliumNodeInformer,
	cache cache2.Cache,
	log logr.Logger,
	tableId int,
	cidrs []*net.IPNet,
	dryRun bool,
) *Manager {
	nlWrapper := netlinkwrap2.NewFuncs()
	if dryRun {
		nlWrapper = netlinkwrap2.DryRunFuncs()
	}
	return &Manager{
		//client:    client,
		vmLister:  vmInformer.Lister(),
		cnIndexer: cnInformer.Informer().GetIndexer(),
		hasSynced: func() bool {
			return vmInformer.Informer().HasSynced() && cnInformer.Informer().HasSynced()
		},
		log:       log.WithValues("manager", netlinkManager),
		tableId:   tableId,
		cidrs:     cidrs,
		nlWrapper: nlWrapper,
		cache:     cache,
	}
}

// SyncRules adds rules for configured CIDRS into the Cilium table.
// Also, it removes existing rules for previously configured CIDRs.
func (m *Manager) SyncRules() error {
	// Get rules state.
	rules, err := m.nlWrapper.RuleListFiltered(netlinkwrap2.FAMILY_ALL, &netlink.Rule{Table: m.tableId}, netlink.RT_FILTER_TABLE)
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
	if !cache.WaitForNamedCacheSync(netlinkManager, ctx.Done(), m.hasSynced) {
		return fmt.Errorf("cache is not synced")
	}
	vms, err := m.vmLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list VirtualMachines: %w", err)
	}

	vmIPsIdx := make(map[string]struct{})

	// Collect managed IPs from all VirtualMachines in the cluster.
	for _, vm := range vms {
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
	nodeRoutes, err := m.nlWrapper.RouteListFiltered(netlinkwrap2.FAMILY_ALL, &netlink.Route{Table: m.tableId}, netlink.RT_FILTER_TABLE)
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
func (m *Manager) UpdateRoute(vm *virtv2.VirtualMachine) error {
	// TODO Add cleanup if node was lost?
	// TODO What about migration? Is nodeName just changed to new node or we need some workarounds when 2 Pods are running?
	if vm.Status.Node == "" {
		// VM has no node assigned
		return nil
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

	// Retrieve a Cilium Node by VMs node name.
	var ciliumNode *ciliumv2.CiliumNode

	obj, exists, err := m.cnIndexer.GetByKey(vm.Status.Node)
	if err != nil {
		m.log.Error(err, "failed to get cilium node for vm")
	}
	if exists {
		ciliumNode = obj.(*ciliumv2.CiliumNode)
	}

	nodeIP := getCiliumInternalIPAddress(ciliumNode)
	if nodeIP == "" {
		return fmt.Errorf("ciliumNode has no %s specified", addressing.NodeCiliumInternalIP)
	}
	nodeIPx := net.ParseIP(nodeIP)
	if len(nodeIPx) == 0 {
		return fmt.Errorf("invalid IP address %s", nodeIP)
	}
	m.cache.Set(vmKey, cache2.Addresses{VMIP: vmIP, NodeIP: nodeIP})

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
	route.Table = m.tableId
	route.Type = 1

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
			vmIP = addr.VMIP
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
		Table: m.tableId,
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
