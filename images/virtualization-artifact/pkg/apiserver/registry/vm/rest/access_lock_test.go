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
	"fmt"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/endpoints/request"

	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
)

type staticAccessLockReader struct {
	lock *vmaccess.Lock
	err  error
}

func (r staticAccessLockReader) ReadAccessLock(context.Context, string, string) (*vmaccess.Lock, error) {
	return r.lock, r.err
}

var _ = Describe("Access lock check", func() {
	ctx := request.WithNamespace(context.Background(), "ns")

	It("lets a call through when the virtual machine is not held", func() {
		base := &BaseREST{accessLocks: staticAccessLockReader{}}
		Expect(base.checkAccessLock(ctx, "vm", "")).To(Succeed())
	})

	It("lets only the holding claim through when the virtual machine is held", func() {
		base := &BaseREST{accessLocks: staticAccessLockReader{lock: &vmaccess.Lock{ClaimUID: "holder"}}}

		Expect(base.checkAccessLock(ctx, "vm", "holder")).To(Succeed())

		err := base.checkAccessLock(ctx, "vm", "")
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
		err = base.checkAccessLock(ctx, "vm", "other")
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
	})

	It("rejects every call while the lock names no claim", func() {
		base := &BaseREST{accessLocks: staticAccessLockReader{err: fmt.Errorf("%w: test", vmaccess.ErrMalformedLock)}}

		err := base.checkAccessLock(ctx, "vm", "")
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
		err = base.checkAccessLock(ctx, "vm", "claim")
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
	})

	It("does not forward the claim UID to KubeVirt", func() {
		req := &http.Request{URL: &url.URL{RawQuery: "claimUID=holder&other=kept"}}
		Expect(dropClaimUIDParam(req)).To(Succeed())
		Expect(req.URL.Query().Has("claimUID")).To(BeFalse())
		Expect(req.URL.Query().Get("other")).To(Equal("kept"))
	})
})
