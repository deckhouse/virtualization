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

package vmpool

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmpoolcondition"
)

// The handlers are tested one by one; this checks the chain as the controller
// wires it: the failure report reaches the status handler and the status is saved.
var _ = Describe("Reconciler", func() {
	It("keeps a creation failure until the replicas are created", func() {
		ctx := context.Background()
		pool := &v1alpha2.VirtualMachinePool{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ci", UID: "pool-uid", Generation: 1},
			Spec: v1alpha2.VirtualMachinePoolSpec{
				Replicas:        ptr.To(int32(3)),
				ScaleDownPolicy: v1alpha2.ScaleDownPolicyNewestFirst,
			},
		}
		key := types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}

		// The quota admits one replica until it is raised.
		quotaLeft := 1
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*v1alpha2.VirtualMachine); ok {
					if quotaLeft == 0 {
						return apierrors.NewForbidden(schema.GroupResource{Resource: "virtualmachines"}, "", errors.New("exceeded quota: vms"))
					}
					quotaLeft--
				}
				return cl.Create(ctx, obj, opts...)
			},
		}, pool)
		Expect(err).NotTo(HaveOccurred())

		exp := expectations.New()
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventfFunc: func(client.Object, string, string, string, ...interface{}) {},
		}
		r := NewReconciler(c, exp, []Handler{
			handler.NewTemplateHandler(c),
			handler.NewSyncHandler(c, exp, recorder),
			handler.NewDisksHandler(c),
			handler.NewStatusHandler(c),
		})
		progressing := func() *metav1.Condition {
			var got v1alpha2.VirtualMachinePool
			Expect(c.Get(ctx, key, &got)).To(Succeed())
			return meta.FindStatusCondition(got.Status.Conditions, vmpoolcondition.TypeProgressing.String())
		}

		By("one replica is created, two are rejected by the quota")
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).To(HaveOccurred())
		cond := progressing()
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
		Expect(cond.Message).To(HavePrefix("Scaling up: 1 of 3 replicas are created. Cannot create a VirtualMachine from the template: the namespace ResourceQuota is exceeded."))

		By("the created replica is not observed yet, so nothing is tried and the failure stays")
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(progressing().Reason).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))

		By("the quota is raised and the replica is observed: the pool converges")
		quotaLeft = 2
		exp.CreationObserved(key.String())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(progressing()).To(BeNil())
	})
})
