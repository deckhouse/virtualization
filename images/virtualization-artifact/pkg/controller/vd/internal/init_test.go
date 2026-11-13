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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("InitHandler", func() {
	It("records the serial derived from the UID of a new disk", func() {
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "root", UID: "vd-uid"}}
		_, err := NewInitHandler().Handle(context.Background(), vd)
		Expect(err).To(MatchError(reconciler.ErrStopHandlerChain))
		Expect(vd.Status.Serial).To(Equal(kvbuilder.GenerateSerial("vd-uid")))
	})

	It("keeps the serial a disk has, such as the one it came with from another cluster", func() {
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "root", UID: "vd-uid"},
			Status:     v1alpha2.VirtualDiskStatus{Serial: "3f2a"},
		}
		_, err := NewInitHandler().Handle(context.Background(), vd)
		Expect(err).To(MatchError(reconciler.ErrStopHandlerChain))
		Expect(vd.Status.Serial).To(Equal("3f2a"))
	})
})
