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

package generate_secret_for_dvcr

import (
	"context"
	"encoding/base64"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestSetDVCRSecrets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DVCR Secrets Suite")
}

var _ = Describe("DVCR Secrets", func() {
	const (
		defaultPasswordBase64 = "dkREU0I1N1JXeVVFd1NwN3VubDA3SHlnWUx3MzlOTlY="                                             // "vDDSB57RWyUEwSp7unl07HygYLw39NNV"
		defaultSaltBase64     = "bldlM01vZjVySlFNc3I2MDVFNDdBM1pYOU9IQ1dnVkY="                                             // "nWe3Mof5rJQMsr605E47A3ZX9OHCWgVF"
		defaultHtpasswdBase64 = "YWRtaW46JDJhJDEwJHZza21wTjVLSERKUlpNU1pWd3RZWU91YmtEcTEueEF2MXVRQkkvSzFHQ1dpNUpxSnF5amdt" // "admin:$2a$10$vskmpN5KHDJRZMSZVwtYYOubkDq1.xAv1uQBI/K1GCWi5JqJqyjgm"
	)
	var (
		dc        *mock.DependencyContainerMock
		snapshots *mock.SnapshotsMock
		values    *mock.PatchableValuesCollectorMock
	)

	prepareValuesGet := func(passwordRW, salt, htpasswd string) {
		values.GetMock.When(passwordRWValuePath).Then(gjson.Result{Type: gjson.String, Str: passwordRW})
		values.GetMock.When(saltValuePath).Then(gjson.Result{Type: gjson.String, Str: salt})
		values.GetMock.When(htpasswdValuePath).Then(gjson.Result{Type: gjson.String, Str: htpasswd})
	}

	setSnapshots := func(snaps ...pkg.Snapshot) {
		snapshots.GetMock.When(dvcrSecrets).Then(snaps)
	}

	newSnapshot := func(passwordRW, salt, htpasswd string) pkg.Snapshot {
		return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*dvcrSecretData)
			Expect(ok).To(BeTrue())

			data.PasswordRW = passwordRW
			data.Salt = salt
			data.Htpasswd = htpasswd
			return nil
		})
	}

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots: snapshots,
			Values:    values,
			DC:        dc,
			Logger:    log.NewNop(),
		}
	}

	BeforeEach(func() {
		dc = mock.NewDependencyContainerMock(GinkgoT())
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		dc = nil
		snapshots = nil
		values = nil
	})

	It("Should set secrets from secret to values", func() {
		prepareValuesGet("", "", "")
		setSnapshots(newSnapshot(defaultPasswordBase64, defaultSaltBase64, defaultHtpasswdBase64))

		values.SetMock.Set(func(path string, v any) {
			value, ok := v.(string)
			Expect(ok).To(BeTrue())

			switch path {
			case passwordRWValuePath:
				Expect(value).To(Equal(defaultPasswordBase64))
			case saltValuePath:
				Expect(value).To(Equal(defaultSaltBase64))
			case htpasswdValuePath:
				Expect(value).To(Equal(defaultHtpasswdBase64))
			default:
				Fail("unexpected path")
			}
		})

		Expect(handlerDVCRSecrets(context.Background(), newInput())).To(Succeed())
	})

	It("Should regenerate all secrets", func() {
		prepareValuesGet("", "", "")
		setSnapshots(newSnapshot("", "", ""))

		var (
			passwordRW string
			htpasswd   string
		)

		values.SetMock.Set(func(path string, v any) {
			valueBase64, ok := v.(string)
			Expect(ok).To(BeTrue())

			bytes, err := base64.StdEncoding.DecodeString(valueBase64)
			Expect(err).ToNot(HaveOccurred())

			value := string(bytes)

			switch path {
			case passwordRWValuePath:
				passwordRW = value
				Expect(value).To(HaveLen(32))
			case saltValuePath:
				Expect(value).To(HaveLen(32))
			case htpasswdValuePath:
				htpasswd = value
				Expect(len(value)).To(BeNumerically(">", 0))
			default:
				Fail("unexpected path")
			}
		})

		Expect(handlerDVCRSecrets(context.Background(), newInput())).To(Succeed())
		Expect(validateHtpasswd(passwordRW, htpasswd)).To(BeTrue())
	})

	It("Should regenerate only salt", func() {
		prepareValuesGet(defaultPasswordBase64, "", defaultHtpasswdBase64)
		setSnapshots(newSnapshot(defaultPasswordBase64, "", defaultHtpasswdBase64))

		values.SetMock.Set(func(path string, v any) {
			valueBase64, ok := v.(string)
			Expect(ok).To(BeTrue())

			bytes, err := base64.StdEncoding.DecodeString(valueBase64)
			Expect(err).ToNot(HaveOccurred())

			value := string(bytes)

			switch path {
			case saltValuePath:
				Expect(value).To(HaveLen(32))
			default:
				Fail("unexpected path")
			}
		})

		Expect(handlerDVCRSecrets(context.Background(), newInput())).To(Succeed())
	})

	DescribeTable("Should regenerate only password and htpasswd", func(passwordRW, htpasswd string) {
		prepareValuesGet(passwordRW, "salt", htpasswd)
		setSnapshots(newSnapshot(passwordRW, "salt", htpasswd))

		bytes, err := base64.StdEncoding.DecodeString(passwordRW)
		Expect(err).ToNot(HaveOccurred())
		passwordRWForValidate := string(bytes)

		bytes, err = base64.StdEncoding.DecodeString(htpasswd)
		Expect(err).ToNot(HaveOccurred())
		htpasswdForValidate := string(bytes)

		values.SetMock.Set(func(path string, v any) {
			valueBase64, ok := v.(string)
			Expect(ok).To(BeTrue())

			bytes, err := base64.StdEncoding.DecodeString(valueBase64)
			Expect(err).ToNot(HaveOccurred())

			value := string(bytes)

			switch path {
			case passwordRWValuePath:
				passwordRWForValidate = value
				Expect(value).To(HaveLen(32))
			case htpasswdValuePath:
				htpasswdForValidate = value
				Expect(len(value)).To(BeNumerically(">", 0))
			default:
				Fail("unexpected path")
			}
		})

		Expect(handlerDVCRSecrets(context.Background(), newInput())).To(Succeed())
		Expect(validateHtpasswd(passwordRWForValidate, htpasswdForValidate)).To(BeTrue())
	},
		Entry("with empty htpasswd and password", "", ""),
		Entry("with empty password", "", defaultHtpasswdBase64),
		Entry("with empty htpasswd", defaultPasswordBase64, ""),
		Entry("with not valid htpasswd", defaultPasswordBase64, "bm90X3ZhbGlkX2h0cGFzc3dkCg=="),
	)
})

var _ = Describe("Generate secrets", func() {
	It("Should generate valid secrets", func() {
		password := alphaNum(32)
		Expect(password).To(HaveLen(32))
		salt := alphaNum(32)
		Expect(salt).To(HaveLen(32))
		htpasswd, err := generateHtpasswd(password)
		Expect(err).ToNot(HaveOccurred())
		Expect(validateHtpasswd(password, htpasswd)).To(BeTrue())
	})
})
