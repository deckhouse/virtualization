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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/utils/ptr"

	virtclient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtfake "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

// callCreate drives the scaleDownWith Create handler with the given targets.
func callCreate(ctx context.Context, c virtclient.Interface, targets ...string) (runtime.Object, error) {
	ctx = genericapirequest.WithNamespace(ctx, ns)
	body := &subresources.VirtualMachinePoolScaleDownWith{Targets: targets}
	return NewScaleDownWithREST(c).Create(ctx, poolName, body, nil, &metav1.CreateOptions{})
}

const (
	ns       = "ci"
	poolName = "web"
	poolUID  = types.UID("pool-uid-1")
)

func newClient(objs ...runtime.Object) virtclient.Interface {
	return virtfake.NewSimpleClientset(objs...)
}

func pool(replicas int32) *v1alpha2.VirtualMachinePool {
	return &v1alpha2.VirtualMachinePool{
		ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: ns, UID: poolUID},
		Spec:       v1alpha2.VirtualMachinePoolSpec{Replicas: ptr.To(replicas)},
	}
}

func memberOf(p *v1alpha2.VirtualMachinePool, name string) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			UID:             types.UID(name + "-uid"),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(p, v1alpha2.VirtualMachinePoolGVK)},
		},
	}
}

// foreignVM belongs to no pool.
func foreignVM(name string) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(name + "-uid")}}
}

func getReplicas(ctx context.Context, c virtclient.Interface) int32 {
	p, err := c.VirtualizationV1alpha2().VirtualMachinePools(ns).Get(ctx, poolName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	return ptr.Deref(p.Spec.Replicas, -1)
}

func vmExists(ctx context.Context, c virtclient.Interface, name string) bool {
	_, err := c.VirtualizationV1alpha2().VirtualMachines(ns).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

var _ = Describe("ScaleDownWith", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = context.Background() })

	It("deletes the targets and decrements replicas", func() {
		p := pool(3)
		c := newClient(p, memberOf(p, "web-a"), memberOf(p, "web-b"), memberOf(p, "web-c"))

		r := NewScaleDownWithREST(c)
		Expect(r.scaleDown(ctx, ns, poolName, []string{"web-a", "web-b"})).To(Succeed())

		Expect(vmExists(ctx, c, "web-a")).To(BeFalse())
		Expect(vmExists(ctx, c, "web-b")).To(BeFalse())
		Expect(vmExists(ctx, c, "web-c")).To(BeTrue())
		Expect(getReplicas(ctx, c)).To(Equal(int32(1)))
	})

	It("rejects a target that does not belong to the pool and deletes nothing", func() {
		p := pool(2)
		c := newClient(p, memberOf(p, "web-a"), foreignVM("intruder"))

		err := NewScaleDownWithREST(c).scaleDown(ctx, ns, poolName, []string{"web-a", "intruder"})
		Expect(apierrors.IsBadRequest(err)).To(BeTrue())

		// Validation happens up front, so no target is deleted and replicas stay.
		Expect(vmExists(ctx, c, "web-a")).To(BeTrue())
		Expect(vmExists(ctx, c, "intruder")).To(BeTrue())
		Expect(getReplicas(ctx, c)).To(Equal(int32(2)))
	})

	It("rejects a missing target", func() {
		p := pool(1)
		c := newClient(p, memberOf(p, "web-a"))

		err := NewScaleDownWithREST(c).scaleDown(ctx, ns, poolName, []string{"ghost"})
		Expect(apierrors.IsBadRequest(err)).To(BeTrue())
	})

	It("floors replicas at zero", func() {
		p := pool(1)
		c := newClient(p, memberOf(p, "web-a"), memberOf(p, "web-b"))

		Expect(NewScaleDownWithREST(c).scaleDown(ctx, ns, poolName, []string{"web-a", "web-b"})).To(Succeed())
		Expect(getReplicas(ctx, c)).To(Equal(int32(0)))
	})

	It("returns NotFound when the pool does not exist", func() {
		c := newClient()

		err := NewScaleDownWithREST(c).scaleDown(ctx, ns, poolName, []string{"web-a"})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	Context("Create handler", func() {
		It("rejects an empty targets list with BadRequest", func() {
			c := newClient(pool(2), memberOf(pool(2), "web-a"))

			_, err := callCreate(ctx, c)
			Expect(apierrors.IsBadRequest(err)).To(BeTrue())
		})

		It("removes the target and reports success on a valid body", func() {
			p := pool(2)
			c := newClient(p, memberOf(p, "web-a"), memberOf(p, "web-b"))

			obj, err := callCreate(ctx, c, "web-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).To(BeAssignableToTypeOf(&metav1.Status{}))
			Expect(vmExists(ctx, c, "web-a")).To(BeFalse())
			Expect(getReplicas(ctx, c)).To(Equal(int32(1)))
		})
	})
})
