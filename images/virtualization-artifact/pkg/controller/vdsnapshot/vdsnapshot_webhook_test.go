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

package vdsnapshot

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Validator", func() {
	DescribeTable("should admit a VirtualDiskSnapshot only with the VolumeSnapshot API",
		func(volumeSnapshotAPIAvailable, wantErr bool) {
			obj := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot", Namespace: "default"},
				Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "vd"},
			}

			_, err := NewValidator(log.NewNop(), volumeSnapshotAPIAvailable).ValidateCreate(context.Background(), obj)
			if wantErr {
				Expect(err).To(MatchError(ContainSubstring("storage-foundation")))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("API available", true, false),
		Entry("API unavailable", false, true),
	)
})
