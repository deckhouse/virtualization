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

package vmip

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	vmips []*v1alpha2.VirtualMachineIPAddress
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vmip := range s.vmips {
		if stop := h(newDataMetric(vmip)); stop {
			return nil
		}
	}
	return nil
}

func newVMIP(name, namespace string, phase v1alpha2.VirtualMachineIPAddressPhase) *v1alpha2.VirtualMachineIPAddress {
	return &v1alpha2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     v1alpha2.VirtualMachineIPAddressStatus{Phase: phase},
	}
}

// sdnServesMainReader answers as a cluster where the sdn module serves the Main network: the
// collector is quiet everywhere else, and the deprecation count is meaningless
// without something to migrate to.
func sdnServesMainReader(available bool) client.Reader {
	scheme := runtime.NewScheme()
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(sdnapi.AddToScheme(scheme)).To(Succeed())

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if available {
		cn := &unstructured.Unstructured{}
		cn.SetGroupVersionKind(network.ClusterNetworkGVK)
		cn.SetName(network.MainClusterNetworkName)
		Expect(unstructured.SetNestedField(cn.Object, "PrimaryCNI", "spec", "type")).To(Succeed())
		Expect(unstructured.SetNestedField(cn.Object, map[string]any{"name": "d8-virtualization"}, "spec", "ipam", "ipAddressPoolRef")).To(Succeed())

		builder = builder.WithObjects(cn)
	}
	return builder.Build()
}

func collectorOf(vmips ...*v1alpha2.VirtualMachineIPAddress) Collector {
	return Collector{
		log:      log.NewNop(),
		reader:   sdnServesMainReader(true),
		iterator: stubIterator{vmips: vmips},
	}
}

var _ = Describe("Collector", func() {
	It("counts resources per namespace and phase", func() {
		c := collectorOf(
			newVMIP("vm-a-ip", "ns-1", v1alpha2.VirtualMachineIPAddressPhaseAttached),
			newVMIP("vm-b-ip", "ns-1", v1alpha2.VirtualMachineIPAddressPhaseAttached),
			newVMIP("vm-c-ip", "ns-1", v1alpha2.VirtualMachineIPAddressPhaseBound),
			newVMIP("vm-d-ip", "ns-2", v1alpha2.VirtualMachineIPAddressPhasePending),
		)

		expected := `
# HELP d8_virtualization_deprecated_vmip_count Number of deprecated VirtualMachineIPAddress resources still present in the cluster. The address of the Main network of a virtual machine is managed by the IPAddress resources of the sdn module; the virtual machines are to be moved off the VirtualMachineIPAddress resources.
# TYPE d8_virtualization_deprecated_vmip_count gauge
d8_virtualization_deprecated_vmip_count{namespace="ns-1",phase="Attached"} 2
d8_virtualization_deprecated_vmip_count{namespace="ns-1",phase="Bound"} 1
d8_virtualization_deprecated_vmip_count{namespace="ns-2",phase="Pending"} 1
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	It("reports no series when there are no VirtualMachineIPAddresses", func() {
		Expect(testutil.CollectAndCount(collectorOf(), fqName(MetricDeprecatedVMIPCount))).To(BeZero())
	})
})

var _ = Describe("Collector in a cluster where the sdn module does not serve the Main network", func() {
	vmip := newVMIP("vmip-1", "ns-1", v1alpha2.VirtualMachineIPAddressPhaseAttached)

	// There is nothing to move the virtual machines to, and the alert over the metric
	// would fire on every cluster with a vmip.
	It("reports no series", func() {
		c := collectorOf(vmip)
		c.reader = sdnServesMainReader(false)
		Expect(testutil.CollectAndCount(c, fqName(MetricDeprecatedVMIPCount))).To(BeZero())
	})
})

var _ = Describe("Collector without the sdn module", func() {
	It("reports nothing: there is nowhere to migrate to", func() {
		c := Collector{
			log:      log.NewNop(),
			reader:   sdnServesMainReader(false),
			iterator: stubIterator{vmips: []*v1alpha2.VirtualMachineIPAddress{newVMIP("vmip", "vms", v1alpha2.VirtualMachineIPAddressPhaseBound)}},
		}

		Expect(testutil.CollectAndCount(c, fqName(MetricDeprecatedVMIPCount))).To(BeZero())
	})
})
