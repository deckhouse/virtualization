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

package peering

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	intvirt "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

var _ = Describe("Migration synchronization", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable("refreshes endpoint annotations without changing the spec or unrelated annotations",
		func(spec virtv1.VirtualMachineInstanceMigrationSpec) {
			migration := &virtv1.VirtualMachineInstanceMigration{
				ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: "ns", Annotations: map[string]string{"other": "keep"}},
				Spec:       spec,
			}
			originalSpec := migration.Spec.DeepCopy()
			scheme := runtime.NewScheme()
			Expect(virtv1.AddToScheme(scheme)).To(Succeed())

			patches := 0
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(migration).
				WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(
						patchCtx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption,
					) error {
						patches++
						return cl.Patch(patchCtx, obj, patch, opts...)
					},
				}).Build()
			key := client.ObjectKeyFromObject(migration)

			endpoint := intvirt.MigrationSynchronizationEndpoint{SyncAddress: "peer:9185", SyncPin: "sha256:first"}
			for range 2 {
				Expect(SetMigrationSynchronization(ctx, c, key, endpoint)).To(Succeed())
			}

			Expect(patches).To(Equal(1), "an unchanged endpoint must not be patched again")

			endpoint = intvirt.MigrationSynchronizationEndpoint{SyncAddress: "new-peer:9185", SyncPin: "sha256:rotated"}
			Expect(SetMigrationSynchronization(ctx, c, key, endpoint)).To(Succeed())

			got := &virtv1.VirtualMachineInstanceMigration{}
			Expect(c.Get(ctx, key, got)).To(Succeed())
			Expect(got.Annotations).To(HaveKeyWithValue(migrationPeerSyncPinAnnotation, endpoint.SyncPin))
			Expect(got.Annotations).To(HaveKeyWithValue(migrationPeerSyncAddressAnnotation, endpoint.SyncAddress))
			Expect(got.Spec).To(Equal(*originalSpec), "endpoint refresh must not change the migration spec")
			Expect(got.Annotations).To(HaveKeyWithValue("other", "keep"))
		},
		Entry("on the source", virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: "vm",
			SendTo:  &virtv1.VirtualMachineInstanceMigrationSource{MigrationID: "id", ConnectURL: "original:9185"},
		}),
		Entry("on the target", virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: "vm",
			Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "id"},
		}),
	)

	Context("published endpoint", func() {
		var (
			c   client.Client
			key client.ObjectKey
		)

		BeforeEach(func() {
			instance := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "kubevirt.io/v1", "kind": "VirtualMachineInstance",
				"metadata": map[string]any{"name": "vm", "namespace": "ns"},
				"status": map[string]any{"migrationState": map[string]any{"sourceState": map[string]any{
					"syncAddress": "source:9185", "syncPin": "sha256:source", "migrationUID": "current",
				}}},
			}}
			c = fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(instance).Build()
			key = client.ObjectKeyFromObject(instance)
		})

		It("returns the endpoint of the current migration", func() {
			endpoint, err := MigrationSynchronization(ctx, c, key, "sourceState", "current")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal(intvirt.MigrationSynchronizationEndpoint{
				SyncAddress: "source:9185", SyncPin: "sha256:source",
			}))
		})

		It("waits for a side that has not published its endpoint", func() {
			endpoint, err := MigrationSynchronization(ctx, c, key, "targetState", "current")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(BeZero(), "an unpublished side must wait")
		})

		DescribeTable("ignores an endpoint without a matching migration identity", func(migrationUID types.UID) {
			endpoint, err := MigrationSynchronization(ctx, c, key, "sourceState", migrationUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(BeZero(), "an endpoint from a different migration must not be published")
		},
			Entry("when the identity is missing", types.UID("")),
			Entry("when the identity belongs to a previous migration", types.UID("previous-migration")),
		)
	})
})
