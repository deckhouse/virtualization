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

package rest

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/tools/cache"

	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newScaleREST(objs ...*v1alpha2.VirtualMachine) *ScaleREST {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, obj := range objs {
		Expect(indexer.Add(obj)).To(Succeed())
	}
	return NewScaleREST(virtlisters.NewVirtualMachineLister(indexer))
}

var _ = Describe("ScaleREST", func() {
	const (
		ns     = "ci"
		vmName = "web"
	)
	ctx := genericapirequest.WithNamespace(context.Background(), ns)

	It("returns a Scale whose selector matches the VM's virt-launcher pod", func() {
		r := newScaleREST(&v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: ns, UID: types.UID("vm-uid-1")},
		})

		obj, err := r.Get(ctx, vmName, &metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		scale, ok := obj.(*autoscalingv1.Scale)
		Expect(ok).To(BeTrue())
		Expect(scale.Name).To(Equal(vmName))
		Expect(scale.Namespace).To(Equal(ns))
		Expect(scale.UID).To(Equal(types.UID("vm-uid-1")))
		// Replicas are a constant contract value; a VM is a single instance.
		Expect(scale.Spec.Replicas).To(Equal(int32(1)))
		Expect(scale.Status.Replicas).To(Equal(int32(1)))

		// The selector must be parseable and match a virt-launcher pod of this VM,
		// since that is the only field the VPA recommender consumes.
		Expect(scale.Status.Selector).ToNot(BeEmpty())
		selector, err := labels.Parse(scale.Status.Selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(selector.Matches(labels.Set{
			launcherAppLabel: "virt-launcher",
			vmNameLabel:      vmName,
		})).To(BeTrue())
		Expect(selector.Matches(labels.Set{
			launcherAppLabel: "virt-launcher",
			vmNameLabel:      "other",
		})).To(BeFalse())
	})

	It("returns NotFound for a missing VM", func() {
		r := newScaleREST()
		_, err := r.Get(ctx, "ghost", &metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
