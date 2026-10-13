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

package mainippool

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/mainippool/internal/handler"
)

type stubHandler struct {
	err    error
	called bool
}

func (h *stubHandler) Handle(context.Context) error {
	h.called = true
	return h.err
}

func (h *stubHandler) Name() string { return "stub" }

var _ = Describe("Reconciler", func() {
	reconcileWith := func(handlers ...Handler) (reconcile.Result, error) {
		return NewReconciler(handlers, nil).Reconcile(context.Background(), reconcile.Request{})
	}

	It("comes back after the resync interval to restore a hand-edited pool", func() {
		first, second := &stubHandler{}, &stubHandler{}

		res, err := reconcileWith(first, second)

		Expect(err).NotTo(HaveOccurred())
		Expect(first.called && second.called).To(BeTrue())
		Expect(res.RequeueAfter).To(Equal(poolResyncInterval))
	})

	It("waits for the sdn kinds without an error", func() {
		noMatch := &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "network.deckhouse.io", Kind: "ClusterIPAddressPool"}}
		next := &stubHandler{}

		res, err := reconcileWith(&stubHandler{err: noMatch}, next)

		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(crdRetryInterval))
		Expect(next.called).To(BeFalse())
	})

	It("waits for a deleted pool to go away without putting its reference back", func() {
		next := &stubHandler{}

		res, err := reconcileWith(&stubHandler{err: fmt.Errorf("handler: %w", handler.ErrPoolTerminating)}, next)

		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(poolTerminatingRetryInterval))
		Expect(next.called).To(BeFalse(), "the sdn module keeps a referenced pool from going away")
	})

	It("returns any other error", func() {
		_, err := reconcileWith(&stubHandler{err: errors.New("boom")})

		Expect(err).To(MatchError(ContainSubstring("boom")))
	})
})
