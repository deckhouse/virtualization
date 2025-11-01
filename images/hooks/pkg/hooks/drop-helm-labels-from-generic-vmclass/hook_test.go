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

package drop_helm_labels_from_generic_vmclass

import (
	"context"
	"fmt"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDropHelmLabelsFromGenericVMClass(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Drop Helm labels from generic VMClass Suite")
}

var _ = Describe("Drop Helm labels from generic VMClass", func() {
	var (
		snapshots      *mock.SnapshotsMock
		patchCollector *mock.PatchCollectorMock
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots:      snapshots,
			PatchCollector: patchCollector,
			Logger:         log.NewNop(),
		}
	}

	newSnapshot := func(withManagedBy, withHeritage bool, withAnnotations bool) pkg.Snapshot {
		return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			obj, ok := v.(*VMClassMetadata)
			Expect(ok).To(BeTrue())
			obj.Name = genericVMClassName
			obj.Labels = make(map[string]string)

			// Required labels for VMClass to be found by the hook
			obj.Labels["app"] = "virtualization-controller"
			obj.Labels["module"] = "virtualization"

			if withManagedBy {
				obj.Labels[helmManagedByLabel] = "Helm"
			}
			if withHeritage {
				obj.Labels[helmHeritageLabel] = "deckhouse"
			}

			if withAnnotations {
				obj.Annotations = make(map[string]string)
				obj.Annotations[helmReleaseNameAnno] = "virtualization"
				obj.Annotations[helmReleaseNamespaceAnno] = "d8-virtualization"
			}

			return nil
		})
	}

	setSnapshots := func(snaps ...pkg.Snapshot) {
		snapshots.GetMock.When(vmClassSnapshot).Then(snaps)
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		patchCollector = mock.NewPatchCollectorMock(GinkgoT())
	})

	It("Should drop both Helm labels and annotations from generic VMClass with all required labels", func() {
		setSnapshots(newSnapshot(true, true, true))
		patchCollector.PatchWithJSONMock.Set(func(patch any, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
			Expect(apiVersion).To(Equal("virtualization.deckhouse.io/v1alpha2"))
			Expect(kind).To(Equal("VirtualMachineClass"))
			Expect(namespace).To(Equal(""))
			Expect(name).To(Equal(genericVMClassName))
			Expect(opts).To(HaveLen(0))

			jsonPatch, ok := patch.([]map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(jsonPatch).To(HaveLen(4))

			// Check first patch (managed-by label)
			Expect(jsonPatch[0]["op"]).To(Equal("remove"))
			Expect(jsonPatch[0]["path"]).To(Equal(fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(helmManagedByLabel))))
			Expect(jsonPatch[0]["value"]).To(BeNil())

			// Check second patch (heritage label)
			Expect(jsonPatch[1]["op"]).To(Equal("remove"))
			Expect(jsonPatch[1]["path"]).To(Equal(fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(helmHeritageLabel))))
			Expect(jsonPatch[1]["value"]).To(BeNil())

			// Check third patch (release-name annotation)
			Expect(jsonPatch[2]["op"]).To(Equal("remove"))
			Expect(jsonPatch[2]["path"]).To(Equal(fmt.Sprintf("/metadata/annotations/%s", jsonPatchEscape(helmReleaseNameAnno))))
			Expect(jsonPatch[2]["value"]).To(BeNil())

			// Check fourth patch (release-namespace annotation)
			Expect(jsonPatch[3]["op"]).To(Equal("remove"))
			Expect(jsonPatch[3]["path"]).To(Equal(fmt.Sprintf("/metadata/annotations/%s", jsonPatchEscape(helmReleaseNamespaceAnno))))
			Expect(jsonPatch[3]["value"]).To(BeNil())
		})

		Expect(handlerDropHelmLabels(context.Background(), newInput())).To(Succeed())
	})

	It("Should do nothing when VMClass doesn't have all required labels", func() {
		// Create a snapshot with VMClass that has only some required labels
		partialLabelSnapshot := mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			obj, ok := v.(*VMClassMetadata)
			Expect(ok).To(BeTrue())
			obj.Name = genericVMClassName
			obj.Labels = make(map[string]string)

			// Only some required labels - VMClass won't be processed
			obj.Labels["app"] = "virtualization-controller"
			obj.Labels["module"] = "virtualization"
			// Missing helmManagedByLabel and helmHeritageLabel

			return nil
		})

		setSnapshots(partialLabelSnapshot)
		Expect(handlerDropHelmLabels(context.Background(), newInput())).To(Succeed())
	})

	It("Should drop only labels when annotations are missing", func() {
		setSnapshots(newSnapshot(true, true, false))
		patchCollector.PatchWithJSONMock.Set(func(patch any, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
			jsonPatch, ok := patch.([]map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(jsonPatch).To(HaveLen(2))

			// Check first patch (managed-by label)
			Expect(jsonPatch[0]["op"]).To(Equal("remove"))
			Expect(jsonPatch[0]["path"]).To(Equal(fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(helmManagedByLabel))))

			// Check second patch (heritage label)
			Expect(jsonPatch[1]["op"]).To(Equal("remove"))
			Expect(jsonPatch[1]["path"]).To(Equal(fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(helmHeritageLabel))))
		})

		Expect(handlerDropHelmLabels(context.Background(), newInput())).To(Succeed())
	})

	It("Should not drop annotations with wrong values", func() {
		wrongAnnotationSnapshot := mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			obj, ok := v.(*VMClassMetadata)
			Expect(ok).To(BeTrue())
			obj.Name = genericVMClassName
			obj.Labels = make(map[string]string)

			// Required labels for VMClass to be found by the hook
			obj.Labels["app"] = "virtualization-controller"
			obj.Labels["module"] = "virtualization"
			obj.Labels[helmManagedByLabel] = "Helm"
			obj.Labels[helmHeritageLabel] = "deckhouse"

			// Annotations with wrong values - should not be removed
			obj.Annotations = make(map[string]string)
			obj.Annotations[helmReleaseNameAnno] = "wrong-module-name"
			obj.Annotations[helmReleaseNamespaceAnno] = "wrong-namespace"

			return nil
		})

		setSnapshots(wrongAnnotationSnapshot)
		patchCollector.PatchWithJSONMock.Set(func(patch any, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
			jsonPatch, ok := patch.([]map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(jsonPatch).To(HaveLen(2))

			// Only labels should be removed, not annotations with wrong values
			Expect(jsonPatch[0]["path"]).To(ContainSubstring("/metadata/labels/"))
			Expect(jsonPatch[1]["path"]).To(ContainSubstring("/metadata/labels/"))
		})

		Expect(handlerDropHelmLabels(context.Background(), newInput())).To(Succeed())
	})

	It("Should do nothing when VMClass not found", func() {
		setSnapshots()
		Expect(handlerDropHelmLabels(context.Background(), newInput())).To(Succeed())
	})

	It("Should do nothing when VMClass exists but doesn't match label selector", func() {
		// Create a snapshot with VMClass that has wrong labels
		wrongLabelSnapshot := mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			obj, ok := v.(*VMClassMetadata)
			Expect(ok).To(BeTrue())
			obj.Name = genericVMClassName
			obj.Labels = make(map[string]string)

			// Wrong labels - VMClass won't be found by the hook
			obj.Labels["app"] = "wrong-app"
			obj.Labels["module"] = "wrong-module"
			obj.Labels[helmManagedByLabel] = "Helm"
			obj.Labels[helmHeritageLabel] = "deckhouse"

			return nil
		})

		setSnapshots(wrongLabelSnapshot)
		Expect(handlerDropHelmLabels(context.Background(), newInput())).To(Succeed())
	})
})
