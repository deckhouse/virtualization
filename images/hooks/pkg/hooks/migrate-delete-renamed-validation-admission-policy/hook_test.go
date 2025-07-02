/*
Copyright 2025 Flant JSC

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

package migrate_delete_renamed_validation_admission_policy

import (
	"context"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestMigrateDeleteRenamedValidationAdmissionPolicy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MigrateDeleteRenamedValidationAdmissionPolicy Suite")
}

var _ = Describe("MigrateDeleteRenamedValidationAdmissionPolicy", func() {
	var (
		pc        *mock.PatchCollectorMock
		snapshots *mock.SnapshotsMock
	)

	setSnapshots := func(snapPolicy, snapBinding pkg.Snapshot) {
		snapshots.GetMock.When(policySnapshotName).Then([]pkg.Snapshot{snapPolicy})
		snapshots.GetMock.When(bindingSnapshotName).Then([]pkg.Snapshot{snapBinding})
	}

	newSnapshotPolicy := func(labels map[string]string) pkg.Snapshot {
		snap := mock.NewSnapshotMock(GinkgoT())
		snap.UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*unstructured.Unstructured)
			Expect(ok).To(BeTrue())
			data.SetName(policySnapshotName)
			data.SetKind("ValidatingAdmissionPolicy")
			data.SetAPIVersion("admissionregistration.k8s.io/v1")
			data.SetLabels(labels)
			return nil
		})
		return snap
	}

	newSnapshotBinding := func(labels map[string]string) pkg.Snapshot {
		snap := mock.NewSnapshotMock(GinkgoT())
		snap.UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*unstructured.Unstructured)
			Expect(ok).To(BeTrue())
			data.SetName(bindingSnapshotName)
			data.SetKind("ValidatingAdmissionPolicyBinding")
			data.SetAPIVersion("admissionregistration.k8s.io/v1")
			data.SetLabels(labels)
			return nil
		})
		return snap
	}

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots:      snapshots,
			PatchCollector: pc,
			Logger:         log.NewNop(),
		}
	}

	BeforeEach(func() {
		pc = mock.NewPatchCollectorMock(GinkgoT())
		snapshots = mock.NewSnapshotsMock(GinkgoT())
	})

	AfterEach(func() {
		pc = nil
		snapshots = nil
	})

	DescribeTable("Check obsolete resources state",
		func(policyLabels map[string]string, policyShouldDelete bool, bindingLabels map[string]string,
			bindingShouldDelete bool,
		) {
			setSnapshots(newSnapshotPolicy(policyLabels), newSnapshotBinding(bindingLabels))

			if policyShouldDelete || bindingShouldDelete {
				pc.DeleteMock.Set(
					func(apiVersion string, kind string, namespace string, name string) {
						labelExist := name == policySnapshotName || name == bindingSnapshotName

						switch kind {
						case "ValidatingAdmissionPolicy":
							Expect(labelExist).To(Equal(policyShouldDelete))
						case "ValidatingAdmissionPolicyBinding":
							Expect(labelExist).To(Equal(bindingShouldDelete))
						default:
							Fail("unexpected kind")
						}
					})
			}

			Expect(reconcile(context.Background(), newInput())).To(Succeed())
		},
		Entry("should not delete VPA VPAB from original kubevirt installation",
			map[string]string{managedByLabel: "virt-operator"},
			false,
			map[string]string{managedByLabel: "virt-operator"},
			false),
		Entry("should not delete VPAB from original kubevirt installation",
			map[string]string{managedByLabel: managedByLabelValue},
			true,
			map[string]string{"app.kubernetes.io/managed-by": "virt-operator"},
			false,
		),
		Entry("should not delete VPA from original kubevirt installation",
			map[string]string{managedByLabel: "virt-operator"},
			false,
			map[string]string{managedByLabel: managedByLabelValue},
			true,
		),
		Entry("should delete non renamed VPA VPAB",
			map[string]string{managedByLabel: managedByLabelValue},
			true,
			map[string]string{managedByLabel: managedByLabelValue},
			true,
		),
	)
})
