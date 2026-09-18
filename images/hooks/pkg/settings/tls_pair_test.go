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

package settings

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"
	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/certificate"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestTLSPair(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TLS Pair Suite")
}

var _ = Describe("TLS pair", func() {
	const secretName = "virtualization-test-tls"

	var (
		ca      *certificate.Authority
		otherCA *certificate.Authority
		pair    *certificate.Certificate
	)

	BeforeEach(func() {
		var err error
		ca, err = certificate.GenerateCA("common-ca")
		Expect(err).ToNot(HaveOccurred())
		otherCA, err = certificate.GenerateCA("other-ca")
		Expect(err).ToNot(HaveOccurred())
		pair, err = certificate.GenerateSelfSignedCert("leaf", ca, certificate.WithSANs("localhost"))
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("VerifyTLSPair", func() {
		It("accepts a leaf signed by its ca.crt", func() {
			Expect(VerifyTLSPair(*pair)).To(Succeed())
		})

		It("rejects a leaf not signed by its ca.crt", func() {
			pair.CA = otherCA.Cert
			Expect(VerifyTLSPair(*pair)).ToNot(Succeed())
		})

		It("rejects a key that does not match the leaf", func() {
			other, err := certificate.GenerateSelfSignedCert("leaf", ca)
			Expect(err).ToNot(HaveOccurred())
			pair.Key = other.Key
			Expect(VerifyTLSPair(*pair)).ToNot(Succeed())
		})

		It("rejects a missing ca.crt", func() {
			pair.CA = nil
			Expect(VerifyTLSPair(*pair)).ToNot(Succeed())
		})
	})

	Describe("ReissueBrokenTLSPair", func() {
		var (
			snapshots *mock.SnapshotsMock
			patches   *mock.PatchCollectorMock
		)

		newInput := func(snaps ...pkg.Snapshot) *pkg.HookInput {
			snapshots = mock.NewSnapshotsMock(GinkgoT())
			patches = mock.NewPatchCollectorMock(GinkgoT())
			snapshots.GetMock.When(tlscertificate.InternalTLSSnapshotKey).Then(snaps)
			return &pkg.HookInput{Snapshots: snapshots, PatchCollector: patches, Logger: log.NewNop()}
		}

		snapshotOf := func(c certificate.Certificate) pkg.Snapshot {
			return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				*(v.(*certificate.Certificate)) = c
				return nil
			})
		}

		It("keeps a consistent secret", func() {
			ReissueBrokenTLSPair(newInput(snapshotOf(*pair)), secretName)
			Expect(patches.DeleteAfterCounter()).To(BeZero())
		})

		It("does nothing without a secret", func() {
			ReissueBrokenTLSPair(newInput(), secretName)
			Expect(patches.DeleteAfterCounter()).To(BeZero())
		})

		It("deletes a secret whose leaf is not signed by its ca.crt", func() {
			pair.CA = otherCA.Cert
			input := newInput(snapshotOf(*pair))
			patches.DeleteMock.Expect("v1", "Secret", ModuleNamespace, secretName).Return()
			ReissueBrokenTLSPair(input, secretName)
			Expect(patches.DeleteAfterCounter()).To(Equal(uint64(1)))
		})
	})
})
