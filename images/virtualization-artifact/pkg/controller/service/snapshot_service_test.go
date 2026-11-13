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

package service

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SnapshotService", func() {
	var (
		ctx         context.Context
		snapshotter *SnapshotService
		clientMock  *ClientMock
	)

	BeforeEach(func() {
		ctx = testContext()
		clientMock = &ClientMock{}
		snapshotter = NewSnapshotService(clientMock, NewProtectionService(clientMock, "finalizer"))
	})

	Context("GetVolumeSnapshot", func() {
		It("treats a missing VolumeSnapshot API as a missing VolumeSnapshot", func() {
			clientMock.GetFunc = func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
				return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "snapshot.storage.k8s.io", Kind: "VolumeSnapshot"}}
			}

			vs, err := snapshotter.GetVolumeSnapshot(ctx, "vs", "ns")
			Expect(err).NotTo(HaveOccurred())
			Expect(vs).To(BeNil())
		})
	})
})
