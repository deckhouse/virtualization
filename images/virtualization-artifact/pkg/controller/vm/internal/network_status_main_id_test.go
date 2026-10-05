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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// The id of the Main network is its ACPI index in the guest, and id 1 is not reserved for
// it: a Main that holds another id must be reported with that id, or the status names an
// interface the guest does not have.
var _ = Describe("NetworkInterfaceHandler reporting the id of the only Main network", func() {
	ctx := testutil.ContextBackgroundWithNoOpLogger()

	statusOf := func(networks []v1alpha2.NetworksSpec) []v1alpha2.NetworksStatus {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-main-id", Namespace: "default", UID: "vm-main-id-uid"},
			Spec:       v1alpha2.VirtualMachineSpec{Networks: networks},
		}
		_, _, vmState := setupEnvironment(vm)
		h := &NetworkInterfaceHandler{virtualMachineCIDRs: []string{"10.0.0.0/24"}}
		changed := vmState.VirtualMachine().Changed()
		_, err := h.UpdateNetworkStatus(ctx, vmState, changed)
		Expect(err).NotTo(HaveOccurred())
		return changed.Status.Networks
	}

	It("reports the id the Main network carries in the spec", func() {
		Expect(statusOf([]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(2)}})).To(ConsistOf(
			v1alpha2.NetworksStatus{ID: 2, Type: v1alpha2.NetworksTypeMain},
		))
	})

	It("reports the first id for the implicit Main of a machine that lists no network", func() {
		Expect(statusOf(nil)).To(ConsistOf(
			v1alpha2.NetworksStatus{ID: commonnetwork.MinID, Type: v1alpha2.NetworksTypeMain},
		))
	})
})
