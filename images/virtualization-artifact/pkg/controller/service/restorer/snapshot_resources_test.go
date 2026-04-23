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

package restorer

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SnapshotResources.Prepare", func() {
	It("maps VirtualMachineMACAddressName by original network index", func() {
		vm := &v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, MAC: "02:00:00:00:00:11"},
				},
			},
		}

		vmmacs := []v1alpha2.VirtualMachineMACAddress{
			{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualMachineMACAddressKind,
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{Name: "vm-mac-secondary", Namespace: "default"},
				Status:     v1alpha2.VirtualMachineMACAddressStatus{Address: "02:00:00:00:00:11"},
			},
		}

		vmJSON, err := json.Marshal(vm)
		Expect(err).NotTo(HaveOccurred())

		vmmacsJSON, err := json.Marshal(vmmacs)
		Expect(err).NotTo(HaveOccurred())

		restorerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "restorer-secret", Namespace: "default"},
			Data: map[string][]byte{
				virtualMachineKey:             vmJSON,
				virtualMachineMACAddressesKey: vmmacsJSON,
			},
		}

		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"},
		}

		resources := NewSnapshotResources(
			fakeClient, v1alpha2.VMOPTypeRestore, v1alpha2.SnapshotOperationModeStrict,
			restorerSecret, vmSnapshot, "restore-uid",
		)
		Expect(resources.Prepare(context.Background())).To(Succeed())

		var restoredVM *v1alpha2.VirtualMachine
		for _, handler := range resources.GetObjectHandlers() {
			if obj, ok := handler.Object().(*v1alpha2.VirtualMachine); ok {
				restoredVM = obj
				break
			}
		}

		Expect(restoredVM).NotTo(BeNil())
		Expect(restoredVM.Spec.Networks[0].VirtualMachineMACAddressName).To(BeEmpty())
		Expect(restoredVM.Spec.Networks[1].VirtualMachineMACAddressName).To(Equal("vm-mac-secondary"))
	})
})
