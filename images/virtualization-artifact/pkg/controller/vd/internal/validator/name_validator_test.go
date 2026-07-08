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

package validator

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("VirtualDisk NameValidator", func() {
	v := NewNameValidator()

	newVD := func(name string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	It("rejects a name containing a dot on create (DVP rule, not enforced by Kubernetes)", func() {
		_, err := v.ValidateCreate(context.Background(), newVD("my.disk"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("'.' is forbidden"))
	})

	It("accepts a long dot-free name on create (length is bounded by Kubernetes, not by DVP)", func() {
		_, err := v.ValidateCreate(context.Background(), newVD("vd-"+strings.Repeat("a", 200)))
		Expect(err).NotTo(HaveOccurred())
	})

	It("warns about a dot on update", func() {
		warnings, err := v.ValidateUpdate(context.Background(), newVD("my.disk"), newVD("my.disk"))
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).NotTo(BeEmpty())
	})
})
