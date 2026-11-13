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

package vmsnapshot

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("Validator", func() {
	DescribeTable("should require the VolumeSnapshot API only for the built-in mechanism",
		func(unifiedSnapshotterPresent, pinBuiltIn, volumeSnapshotAPIAvailable, wantErr bool) {
			vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vmsnapshot", Namespace: "default"},
				Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"},
			}
			if pinBuiltIn {
				vmSnapshot.Annotations = map[string]string{v1alpha2.AnnUseBuiltInSnapshotter: ""}
			}

			c, err := testutil.NewFakeClientWithObjects(&v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = NewValidator(c, unifiedSnapshotterPresent, volumeSnapshotAPIAvailable).ValidateCreate(context.Background(), vmSnapshot)
			if wantErr {
				Expect(err).To(MatchError(ContainSubstring("storage-foundation")))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("built-in with the API", false, false, true, false),
		Entry("built-in without the API", false, false, false, true),
		Entry("unified without the API", true, false, false, false),
		Entry("pinned to built-in without the API", true, true, false, true),
	)

	DescribeTable("validates the peer migration lock before allowing a snapshot",
		func(condition string, marked bool, wantError string) {
			vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"}}
			if condition != "" {
				vm.Status.Conditions = []metav1.Condition{{Type: condition, Status: metav1.ConditionTrue}}
			}

			kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: vm.Name, Namespace: vm.Namespace}}
			if marked {
				kvvm.Annotations = map[string]string{annotations.AnnPeerMigrationVMOP: "move"}
			}

			c, err := testutil.NewFakeClientWithObjects(vm, kvvm)
			Expect(err).NotTo(HaveOccurred())

			vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: vm.Namespace},
				Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: vm.Name},
			}

			_, err = NewValidator(c, false, true).ValidateCreate(context.Background(), vmSnapshot)
			if wantError == "" {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(MatchError(ContainSubstring(wantError)))
			}
		},
		Entry("allows an ordinary VM", "", false, ""),
		Entry("rejects an outgoing migration", vmcondition.TypeMovedToPeer.String(), false, "being migrated to a peer cluster"),
		Entry("rejects an incoming migration", vmcondition.TypeReceivingFromPeer.String(), false, "being migrated from a peer cluster"),
		Entry("rejects an outgoing migration before its condition is set", "", true, "being migrated to a peer cluster"),
	)
})
