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

package drop_openshift_labels

import (
	"context"
	"fmt"
	"testing"

	"hooks/pkg/settings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestDropOpenshiftLabels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Drop openshift labels Suite")
}

var _ = Describe("Drop openshift labels", func() {
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

	newSnapshot := func(with bool) pkg.Snapshot {
		return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			obj, ok := v.(*NamespaceMetadata)
			Expect(ok).To(BeTrue())
			obj.Name = settings.ModuleNamespace
			if with {
				if obj.Labels == nil {
					obj.Labels = make(map[string]string)
				}
				obj.Labels[openshiftClusterMonitoringLabel] = "true"
			}
			return nil
		})
	}

	setSnapshots := func(snaps ...pkg.Snapshot) {
		snapshots.GetMock.When(moduleNamespace).Then(snaps)
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		patchCollector = mock.NewPatchCollectorMock(GinkgoT())
	})

	It("Should drop label", func() {
		setSnapshots(newSnapshot(true))
		patchCollector.PatchWithJSONMock.Set(func(patch any, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
			Expect(apiVersion).To(Equal("v1"))
			Expect(kind).To(Equal("Namespace"))
			Expect(namespace).To(Equal(""))
			Expect(name).To(Equal(settings.ModuleNamespace))
			Expect(opts).To(HaveLen(0))

			jsonPatch, ok := patch.([]map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(jsonPatch).To(HaveLen(1))
			Expect(jsonPatch[0]["op"]).To(Equal("remove"))
			Expect(jsonPatch[0]["path"]).To(Equal(fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(openshiftClusterMonitoringLabel))))
			Expect(jsonPatch[0]["value"]).To(BeNil())
		})

		Expect(handlerModuleNamespace(context.Background(), newInput())).To(Succeed())
	})
	It("Should do nothing", func() {
		setSnapshots(newSnapshot(false))
		Expect(handlerModuleNamespace(context.Background(), newInput())).To(Succeed())
	})
})
