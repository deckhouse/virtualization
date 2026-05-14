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

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// migrationIfaceAnnotation must match the constant in pkg/common/annotations.
const migrationIfaceAnnotation = "virtualization.deckhouse.io/migration-iface"

var (
	systemNetworkGVK = schema.GroupVersionKind{
		Group:   "network.deckhouse.io",
		Version: "v1alpha1",
		Kind:    "SystemNetwork",
	}
	clusterIPAddressPoolGVK = schema.GroupVersionKind{
		Group:   "network.deckhouse.io",
		Version: "v1alpha1",
		Kind:    "ClusterIPAddressPool",
	}
)

var _ = Describe("VirtualMachineMigrationDedicatedNetwork", Label(precheck.PrecheckSDN), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		systemNetworkName string
		poolCIDRs         []*net.IPNet

		vdRoot *v1alpha2.VirtualDisk
		vmObj  *v1alpha2.VirtualMachine
		vmop   *v1alpha2.VirtualMachineOperation
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-migration-dedicated-network")
		DeferCleanup(f.After)

		f.Before()
	})

	It("routes live migration traffic over the configured SystemNetwork", func() {
		By("Reading liveMigration.systemNetworkName from the virtualization ModuleConfig", func() {
			systemNetworkName = getConfiguredSystemNetworkName(ctx, f)
			if systemNetworkName == "" {
				Skip("ModuleConfig virtualization has no spec.settings.liveMigration.systemNetworkName; configure it to run this test")
			}
		})

		By(fmt.Sprintf("Verifying SystemNetwork %q is Ready and discovering its IPAM pool CIDRs", systemNetworkName), func() {
			sn := &unstructured.Unstructured{}
			sn.SetGroupVersionKind(systemNetworkGVK)
			err := f.GenericClient().Get(ctx, crclient.ObjectKey{Name: systemNetworkName}, sn)
			Expect(err).NotTo(HaveOccurred(),
				"SystemNetwork %q is referenced by the ModuleConfig but not present on the cluster",
				systemNetworkName)
			Expect(isSystemNetworkReady(sn)).To(BeTrue(),
				"SystemNetwork %q must be Ready", systemNetworkName)

			poolName, found, err := unstructured.NestedString(sn.Object, "spec", "ipam", "clusterIPAddressPoolName")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue(),
				"SystemNetwork %q must declare spec.ipam.clusterIPAddressPoolName for IP-pool verification",
				systemNetworkName)

			poolCIDRs = getClusterIPAddressPoolCIDRs(ctx, f, poolName)
			Expect(poolCIDRs).NotTo(BeEmpty(),
				"ClusterIPAddressPool %q has no parsable spec.pools[].network entries", poolName)
		})

		By("Waiting for migration-iface annotations to populate on every node", func() {
			Eventually(func(g Gomega) {
				nodes := &corev1.NodeList{}
				g.Expect(f.GenericClient().List(ctx, nodes)).To(Succeed())
				g.Expect(nodes.Items).NotTo(BeEmpty())
				for _, n := range nodes.Items {
					if !nodeIsReady(&n) {
						continue
					}
					g.Expect(n.Annotations).To(HaveKey(migrationIfaceAnnotation),
						"node %q must carry the migration-iface annotation", n.Name)
					g.Expect(n.Annotations[migrationIfaceAnnotation]).NotTo(BeEmpty(),
						"node %q annotation must not be empty", n.Name)
				}
			}).WithPolling(2 * time.Second).WithTimeout(framework.MiddleTimeout).Should(Succeed())
		})

		By("Creating a VM", func() {
			vdRoot = object.NewVDFromCVI("vd-root-alpine", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
				vd.WithSize(ptr.To(resource.MustParse("10Gi"))),
			)
			vmObj = object.NewMinimalVM("vm-migration-dn", f.Namespace().Name,
				vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdRoot.Name,
				}),
				vm.WithCPU(1, ptr.To("100%")),
				vm.WithBootloader(v1alpha2.BIOS),
				vm.WithProvisioningUserData(object.AlpineCloudInit),
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
			)
			Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, vmObj)).To(Succeed())
			util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vmObj)
			util.UntilSSHReady(f, vmObj, framework.LongTimeout)
		})

		By("Triggering live migration via VMOP", func() {
			vmop = vmopbuilder.New(
				vmopbuilder.WithGenerateName("vmop-migrate-dn-"),
				vmopbuilder.WithNamespace(f.Namespace().Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
				vmopbuilder.WithVirtualMachine(vmObj.Name),
			)
			Expect(f.CreateWithDeferredDeletion(ctx, vmop)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmObj), vmObj)).To(Succeed())
				util.SkipIfKnownMigrationFailure(vmObj)

				g.Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmop), vmop)).To(Succeed())
				g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
			}).WithPolling(time.Second).WithTimeout(framework.LongTimeout).Should(Succeed())
		})

		By("Verifying the migration target address is in the SystemNetwork IPAM pool", func() {
			intvmi := &rewrite.VirtualMachineInstance{}
			Expect(framework.GetClients().RewriteClient().Get(
				ctx, vmObj.Name, intvmi, rewrite.InNamespace(f.Namespace().Name),
			)).To(Succeed())

			state := intvmi.Status.MigrationState
			Expect(state).NotTo(BeNil(), "VMI must have a MigrationState after a completed migration")
			Expect(state.Completed).To(BeTrue())
			Expect(state.TargetNode).NotTo(BeEmpty())
			Expect(state.TargetNodeAddress).NotTo(BeEmpty(),
				"target node address must be advertised by virt-handler")

			ip := net.ParseIP(state.TargetNodeAddress)
			Expect(ip).NotTo(BeNil(),
				"target node address %q is not a parsable IP", state.TargetNodeAddress)
			Expect(cidrsContain(poolCIDRs, ip)).To(BeTrue(),
				"migration target address %s is not within SystemNetwork %q IPAM pool CIDRs %v - feature did not route over SystemNetwork",
				state.TargetNodeAddress, systemNetworkName, formatCIDRs(poolCIDRs))

			node := &corev1.Node{}
			Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Name: state.TargetNode}, node)).To(Succeed())
			Expect(node.Annotations[migrationIfaceAnnotation]).NotTo(BeEmpty(),
				"target node %q has no migration-iface annotation", state.TargetNode)
		})
	})
})

func getConfiguredSystemNetworkName(ctx context.Context, f *framework.Framework) string {
	GinkgoHelper()
	mc, err := f.GetModuleConfig(ctx, "virtualization")
	if k8serrors.IsNotFound(err) {
		return ""
	}
	Expect(err).NotTo(HaveOccurred())

	raw, err := json.Marshal(mc.Spec.Settings)
	Expect(err).NotTo(HaveOccurred())
	settings := map[string]any{}
	Expect(json.Unmarshal(raw, &settings)).To(Succeed())

	lm, ok := settings["liveMigration"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := lm["systemNetworkName"].(string)
	return name
}

func isSystemNetworkReady(obj *unstructured.Unstructured) bool {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		s, _ := m["status"].(string)
		if t == "Ready" && s == "True" {
			return true
		}
	}
	return false
}

func nodeIsReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func getClusterIPAddressPoolCIDRs(ctx context.Context, f *framework.Framework, poolName string) []*net.IPNet {
	GinkgoHelper()
	pool := &unstructured.Unstructured{}
	pool.SetGroupVersionKind(clusterIPAddressPoolGVK)
	Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Name: poolName}, pool)).To(Succeed(),
		"ClusterIPAddressPool %q must exist", poolName)

	pools, found, err := unstructured.NestedSlice(pool.Object, "spec", "pools")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue(), "ClusterIPAddressPool %q must have spec.pools", poolName)

	cidrs := make([]*net.IPNet, 0, len(pools))
	for _, p := range pools {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		network, _ := m["network"].(string)
		if network == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(network)
		if err != nil {
			continue
		}
		cidrs = append(cidrs, ipnet)
	}
	return cidrs
}

func cidrsContain(cidrs []*net.IPNet, ip net.IP) bool {
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

func formatCIDRs(cidrs []*net.IPNet) []string {
	out := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, c.String())
	}
	return out
}
