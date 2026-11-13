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

package vm

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

var _ = Describe("migration receiver webhook", func() {
	DescribeTable("mayCreateReceiver",
		func(name string, groups []string, want bool) { Expect(mayCreateReceiver(name, groups)).To(Equal(want)) },
		Entry("peer of the virtualization capability",
			"peering:peer:cluster-a:system:serviceaccount:d8-virtualization:kubevirt-internal-virtualization-synchronization-controller",
			[]string{"peering:peer:cluster-a:cap:virtualization", "system:authenticated"}, true),
		Entry("own controller", "system:serviceaccount:d8-virtualization:virtualization-controller", nil, true),
		Entry("peer of another capability", "peering:peer:cluster-a:system:serviceaccount:d8-mesh:mesh",
			[]string{"peering:peer:cluster-a:cap:mesh"}, false),
		Entry("no peer name", "peering:peer::system:serviceaccount:ns:sa", []string{"peering:peer::cap:virtualization"}, false),
		Entry("capability only in the user name", "peering:peer:cluster-a:cap:virtualization", nil, false),
		Entry("user", "daniil@flant.com", []string{"system:authenticated"}, false),
		Entry("other service account", "system:serviceaccount:default:daniil", nil, false),
	)

	receiver := func(vm, instance string) *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web-01"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(vm)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(instance)},
				IPAddress: "10.66.10.13",
			},
		}
	}
	DescribeTable("validateReceiver",
		func(vm, instance string, ok bool) {
			err := validateReceiver(receiver(vm, instance))
			if ok {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("a VM with its domain", `{"spec":{"virtualMachineClassName":"generic"}}`, `{"spec":{"domain":{"devices":{"disks":[{"name":"vd-root"}]}}}}`, true),
		Entry("a VM of another name", `{"metadata":{"name":"db-01"}}`, `{"spec":{"domain":{"devices":{}}}}`, false),
		Entry("a host device", `{}`, `{"spec":{"domain":{"devices":{"hostDevices":[{"name":"x","deviceName":"y"}]}}}}`, false),
		Entry("a GPU", `{}`, `{"spec":{"domain":{"devices":{"gpus":[{"name":"x","deviceName":"y"}]}}}}`, false),
		Entry("not JSON", `{`, `{}`, false),
	)

	DescribeTable("MigrationReceiverDefaulter names the peer that writes the request",
		func(groups []string, before, want string) {
			r := receiver(`{}`, `{}`)
			r.Spec.SourcePeer = before
			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UserInfo: authnv1.UserInfo{Username: "u", Groups: groups},
			}})
			Expect(NewMigrationReceiverDefaulter().Default(ctx, r)).To(Succeed())
			Expect(r.Spec.SourcePeer).To(Equal(want))
		},
		Entry("a peer", []string{"system:authenticated", "peering:peer:cluster-a:cap:virtualization"}, "", "cluster-a"),
		Entry("a peer that names another one", []string{"peering:peer:cluster-a:cap:virtualization"},
			"cluster-z", "cluster-a"),
		Entry("the controller here keeps the name", nil, "cluster-a", "cluster-a"),
		Entry("a peer of another capability", []string{"peering:peer:cluster-a:cap:mesh"}, "", ""),
		Entry("no peer name", []string{"peering:peer::cap:virtualization"}, "", ""),
	)

	// The finalizer keeps the request until the objects created for it no longer refer to it.
	DescribeTable("ValidateUpdate of the finalizers and the owner references",
		func(user string, groups []string, change func(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver), ok bool) {
			oldR := receiver(`{}`, `{}`)
			oldR.Finalizers = []string{"internal.virtualization.deckhouse.io/vmr-cleanup"}
			newR := oldR.DeepCopy()
			change(newR)

			ctx := admission.NewContextWithRequest(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				UserInfo: authnv1.UserInfo{Username: user, Groups: groups},
			}})

			_, err := NewMigrationReceiverValidator().ValidateUpdate(ctx, oldR, newR)
			if ok {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(MatchError(ContainSubstring("cannot be changed by a peer cluster")))
			}
		},
		Entry("a peer removes the finalizer", "peering:peer:cluster-a:sa", []string{"peering:peer:cluster-a:cap:virtualization"},
			func(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) { r.Finalizers = nil }, false),
		Entry("a peer adds an owner", "peering:peer:cluster-a:sa", []string{"peering:peer:cluster-a:cap:virtualization"},
			func(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) {
				r.OwnerReferences = []metav1.OwnerReference{{Kind: "Namespace", Name: "x", UID: "u"}}
			}, false),
		Entry("a peer refreshes the source endpoint", "peering:peer:cluster-a:sa", []string{"peering:peer:cluster-a:cap:virtualization"},
			func(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) {
				r.Spec.SourceSynchronization.SyncPin = "sha256:A"
			}, true),
		Entry("the controller here removes the finalizer", "system:serviceaccount:d8-virtualization:virtualization-controller", nil,
			func(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) { r.Finalizers = nil }, true),
	)
	It("refreshes only synchronization inputs and keeps identity and templates immutable", func() {
		oldR := receiver(`{}`, `{}`)
		oldR.Spec.SourcePeer = "a"
		ctx := admission.NewContextWithRequest(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authnv1.UserInfo{Username: ownControllerUser}}})
		updated := oldR.DeepCopy()
		updated.Spec.SourceSynchronization = intvirtv1alpha1.MigrationSynchronizationEndpoint{SyncAddress: "new:9185", SyncPin: "sha256:new"}
		_, err := NewMigrationReceiverValidator().ValidateUpdate(ctx, oldR, updated)
		Expect(err).NotTo(HaveOccurred())

		updated.Spec.SourcePeer = "b"
		_, err = NewMigrationReceiverValidator().ValidateUpdate(ctx, oldR, updated)
		Expect(err).To(HaveOccurred())

		updated = oldR.DeepCopy()
		updated.Spec.VirtualMachineTemplate.Raw = []byte(`{"spec":{"runPolicy":"Manual"}}`)
		_, err = NewMigrationReceiverValidator().ValidateUpdate(ctx, oldR, updated)
		Expect(err).To(HaveOccurred())
	})
})
