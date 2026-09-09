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

package service

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("ProtectionService", func() {
	const finalizer = "virtualization.deckhouse.io/vd-protection"

	newVD := func() *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualDiskKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vd-root",
				Namespace: "default",
			},
		}
	}

	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	Context("AddProtection", func() {
		It("adds the finalizer to a live object", func() {
			vd := newVD()
			fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(vd).Build()
			s := NewProtectionService(fakeClient, finalizer)

			Expect(s.AddProtection(testContext(), vd)).To(Succeed())

			fresh := newVD()
			Expect(fakeClient.Get(testContext(), client.ObjectKeyFromObject(vd), fresh)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fresh, finalizer)).To(BeTrue())
		})

		It("tolerates a 422 when the object started deleting after the cached copy was taken", func() {
			// The cached copy carries no deletionTimestamp, so the in-memory check
			// passes; the API server, however, already sees the object terminating
			// and rejects the finalizer patch with a 422.
			vd := newVD()
			fakeClient := fake.NewClientBuilder().
				WithScheme(newScheme()).
				WithObjects(vd).
				WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(_ context.Context, _ client.WithWatch, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
						return apierrors.NewInvalid(
							v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualDiskKind).GroupKind(),
							obj.GetName(),
							field.ErrorList{field.Forbidden(
								field.NewPath("metadata", "finalizers"),
								`no new finalizers can be added if the object is being deleted, found new finalizers []string{"`+finalizer+`"}`,
							)},
						)
					},
				}).
				Build()
			s := NewProtectionService(fakeClient, finalizer)

			Expect(s.AddProtection(testContext(), vd)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(vd, finalizer)).To(BeFalse(), "the local copy must not pretend the finalizer was added")
		})

		It("tolerates the object being already gone", func() {
			vd := newVD()
			fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
			s := NewProtectionService(fakeClient, finalizer)

			Expect(s.AddProtection(testContext(), vd)).To(Succeed())
		})

		It("still fails on other patch errors", func() {
			vd := newVD()
			fakeClient := fake.NewClientBuilder().
				WithScheme(newScheme()).
				WithObjects(vd).
				WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
						return apierrors.NewServiceUnavailable("boom")
					},
				}).
				Build()
			s := NewProtectionService(fakeClient, finalizer)

			Expect(s.AddProtection(testContext(), vd)).NotTo(Succeed())
		})
	})
})
