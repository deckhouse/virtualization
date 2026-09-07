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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualMachine keeps a DVP-specific name limit of 63 (its name flows into pod
// names and label values); Kubernetes allows up to 253, so DVP must enforce it.
var _ = Describe("VirtualMachine MetaValidator name length", func() {
	featureGate, _, err := featuregates.NewUnlocked()
	if err != nil {
		panic(err)
	}
	v := validators.NewMetaValidator(nil, featureGate)

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

var _ = Describe("VirtualMachine MetaValidator vIOMMU annotation", func() {
	newVM := func(anno map[string]string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Annotations: anno}}
	}
	viommuOn := map[string]string{annotations.AnnEnableVIOMMU: "true"}

	newValidator := func(gateEnabled bool) *validators.MetaValidator {
		featureGate, setFromMap, err := featuregates.NewUnlocked()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{string(featuregates.VIOMMU): gateEnabled})).To(Succeed())
		return validators.NewMetaValidator(nil, featureGate)
	}

	It("rejects creating a VM with the annotation while the gate is off", func() {
		_, err := newValidator(false).ValidateCreate(context.Background(), newVM(viommuOn))
		Expect(err).To(HaveOccurred())
	})

	It("accepts creating a VM with the annotation when the gate is on", func() {
		_, err := newValidator(true).ValidateCreate(context.Background(), newVM(viommuOn))
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects adding the annotation on update while the gate is off", func() {
		_, err := newValidator(false).ValidateUpdate(context.Background(), newVM(nil), newVM(viommuOn))
		Expect(err).To(HaveOccurred())
	})

	It("keeps a VM that already carries the annotation updatable with the gate off", func() {
		_, err := newValidator(false).ValidateUpdate(context.Background(), newVM(viommuOn), newVM(viommuOn))
		Expect(err).NotTo(HaveOccurred())
	})
})
