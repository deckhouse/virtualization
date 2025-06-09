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

package main

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMigrateDeleteRenamedValidationAadmissionPolicy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MigrateDeleteRenamedValidationAadmissionPolicy Suite")
}

var _ = Describe("TST vap", func() {
	var (
		dc        *mock.DependencyContainerMock
		snapshots *mock.SnapshotsMock
	)

	testDataVap := admissionregistrationv1.ValidatingAdmissionPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ValidatingAdmissionPolicy",
			APIVersion: "admissionregistration.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-vap",
			Labels: map[string]string{
				"test": "test",
			},
		},
	}

	setSnapshots := func(snaps ...pkg.Snapshot) {
		snapshots.GetMock.When(POLICY_SNAPSHOT_NAME).Then(snaps...)
	}

	newSnapshot := func(data admissionregistrationv1.ValidatingAdmissionPolicy) pkg.Snapshot {
		return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*admissionregistrationv1.ValidatingAdmissionPolicy)
			Expect(ok).To(BeTrue())
			data.Kind = "ValidatingAdmissionPolicy"
			data.APIVersion = "admissionregistration.k8s.io/v1"
			data.Labels = map[string]string{
				"test": "test",
			}
			return nil
		})
	}

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots: snapshots,
			DC:        dc,
			Logger:    log.NewNop(),
		}
	}
	It("Test", func() {
		setSnapshots(newSnapshot(testDataVap))
		Expect(reconcile(context.Background(), newInput())).To(Succeed())
	})
})

// newSnapshot := func(testDataVap *admissionregistrationv1.ValidatingAdmissionPolicy) *pkg.Snapshot {
// 	return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
// 		data, ok := v.(*admissionregistrationv1.ValidatingAdmissionPolicy)
// 		Expect(ok).To(BeTrue())

// 		*data = *testDataVap
// 		return nil
// 	})
// }

// var _ = Describe("DeleteVAP", func() {
// 	var (
// 		ctx        = testutil.ContextBackgroundWithNoOpLogger()
// 		fakeClient client.WithWatch
// 	)

// 	BeforeEach(func() {
// 		fakeClient = nil
// 	})

// 	newSnapshot := func(passwordRW, salt, htpasswd string) pkg.Snapshot {
// 		return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
// 			data, ok := v.(*dvcrSecretData)
// 			Expect(ok).To(BeTrue())

// 			data.PasswordRW = passwordRW
// 			data.Salt = salt
// 			data.Htpasswd = htpasswd
// 			return nil
// 		})
// 	}

// 	newInput := func() *pkg.HookInput {
// 		return &pkg.HookInput{
// 			Snapshots: snapshots,
// 			Values:    values,
// 			DC:        dc,
// 			Logger:    log.NewNop(),
// 		}
// 	}
// })
