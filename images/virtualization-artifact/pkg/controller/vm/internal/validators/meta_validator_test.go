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

package validators_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualMachine keeps a DVP-specific name limit of 63 (its name flows into pod
// names and label values); Kubernetes allows up to 253, so DVP must enforce it.
var _ = Describe("VirtualMachine MetaValidator name length", func() {
	v := validators.NewMetaValidator(nil)

	newVM := func(name string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	It("accepts a 63-character name", func() {
		_, err := v.ValidateCreate(context.Background(), newVM(strings.Repeat("a", 63)))
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a 64-character name", func() {
		_, err := v.ValidateCreate(context.Background(), newVM(strings.Repeat("a", 64)))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no more than 63 characters"))
	})
})
