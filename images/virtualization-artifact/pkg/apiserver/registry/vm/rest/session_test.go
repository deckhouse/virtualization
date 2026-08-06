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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

var _ = Describe("sessionManager", func() {
	var (
		kubeCli  *fake.Clientset
		recorder *record.FakeRecorder
		manager  *sessionManager
		vm       *v1alpha2.VirtualMachine
	)

	asUser := func(name string) context.Context {
		return request.WithUser(context.Background(), &user.DefaultInfo{Name: name})
	}

	leaseOf := func() *coordinationv1.Lease {
		lease, err := kubeCli.CoordinationV1().Leases(vm.Namespace).Get(context.Background(), sessionLeaseName(vm, subv1alpha2.ConsoleSession), metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		return lease
	}

	currentOf := func() *sessionInfo {
		current, err := manager.Current(context.Background(), vm, subv1alpha2.ConsoleSession)
		Expect(err).NotTo(HaveOccurred())
		return current
	}

	holderOf := func() string {
		current := currentOf()
		if current == nil {
			return ""
		}
		return current.Holder
	}

	BeforeEach(func() {
		kubeCli = fake.NewSimpleClientset()
		recorder = record.NewFakeRecorder(10)
		manager = newSessionManager(kubeCli.CoordinationV1(), recorder)
		vm = &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"}}
	})

	It("reports a free console when nobody is connected", func() {
		Expect(currentOf()).To(BeNil())
	})

	It("reports the connected user, the client and when the session started", func() {
		_, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "d8/v1.2.3")
		Expect(err).NotTo(HaveOccurred())

		current := currentOf()
		Expect(current).NotTo(BeNil())
		Expect(current.Holder).To(Equal("alice"))
		Expect(current.Client).To(Equal("d8/v1.2.3"))
		Expect(current.StartTime).NotTo(BeNil())
		Expect(current.StartTime.Time).To(BeTemporally("~", time.Now(), time.Minute))
	})

	It("lets another user connect and reports the takeover", func() {
		_, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())

		_, err = manager.Acquire(asUser("bob"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(holderOf()).To(Equal("bob"))
		Expect(recorder.Events).To(Receive(SatisfyAll(
			ContainSubstring("ConsoleSessionPreempted"),
			ContainSubstring(`"alice"`),
		)))
	})

	It("does not report a takeover when the same user reconnects", func() {
		_, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())

		_, err = manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.Events).NotTo(Receive())
	})

	It("reports a session that is no longer renewed as free, so it cannot stay stuck", func() {
		_, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())

		stale := leaseOf()
		expired := metav1.NewMicroTime(time.Now().Add(-2 * sessionLeaseDuration))
		stale.Spec.RenewTime = &expired
		_, err = kubeCli.CoordinationV1().Leases(vm.Namespace).Update(context.Background(), stale, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(holderOf()).To(BeEmpty())

		// Taking over an expired session displaces nobody.
		_, err = manager.Acquire(asUser("bob"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.Events).NotTo(Receive())
	})

	It("frees the session on release", func() {
		session, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())
		session.release(context.Background())

		Expect(holderOf()).To(BeEmpty())
	})

	It("does not release a session that was taken over", func() {
		session, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())

		_, err = manager.Acquire(asUser("bob"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())

		Expect(session.renew(context.Background())).To(Succeed())
		session.release(context.Background())
		Expect(holderOf()).To(Equal("bob"))
	})

	It("keeps the console and the VNC sessions apart", func() {
		_, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "")
		Expect(err).NotTo(HaveOccurred())

		_, err = manager.Acquire(asUser("bob"), vm, subv1alpha2.VNCSession, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(holderOf()).To(Equal("alice"))
	})

	It("records the client of the request and forgets the session once it ends", func() {
		base := &BaseREST{sessions: manager}
		handler := base.trackSession(vm, subv1alpha2.ConsoleSession, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			current := currentOf()
			Expect(current).NotTo(BeNil())
			Expect(current.Holder).To(Equal("alice"))
			Expect(current.Client).To(Equal("console/v1.57.0 (linux/amd64)"))
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(asUser("alice"))
		req.Header.Set("User-Agent", "console/v1.57.0 (linux/amd64)")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		Expect(currentOf()).To(BeNil())
	})

	It("keeps renewing the session after the deadline of the request has passed", func() {
		// A stream subresource is not long running for the apiserver, so its request carries a
		// deadline while the stream itself, served over a hijacked connection, outlives it. A
		// session that stopped being renewed there would be reported as free to everybody asking,
		// with somebody still connected.
		sessionRenewInterval = 10 * time.Millisecond
		DeferCleanup(func() { sessionRenewInterval = 10 * time.Second })

		ctx, deadlineReached := context.WithCancel(asUser("alice"))
		base := &BaseREST{sessions: manager}
		handler := base.trackSession(vm, subv1alpha2.ConsoleSession, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			acquired := leaseOf().Spec.RenewTime.Time
			deadlineReached()

			Eventually(func() time.Time { return leaseOf().Spec.RenewTime.Time }).Should(BeTemporally(">", acquired))
			Expect(holderOf()).To(Equal("alice"))
		}))

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	})

	It("cuts an over-long client name by characters, not by bytes", func() {
		base := &BaseREST{sessions: manager}
		handler := base.trackSession(vm, subv1alpha2.ConsoleSession, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			client := currentOf().Client
			Expect(utf8.ValidString(client)).To(BeTrue(), "a character must not be cut in half")
			Expect([]rune(client)).To(HaveLen(maxSessionClientLen))
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(asUser("alice"))
		// A two-byte rune, so that a cut by bytes would land in the middle of a character.
		req.Header.Set("User-Agent", strings.Repeat("é", maxSessionClientLen+10))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	})

	It("leaves a session of a request without an authenticated user untracked", func() {
		connected := false
		base := &BaseREST{sessions: manager}
		handler := base.trackSession(vm, subv1alpha2.ConsoleSession, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			connected = true
			// Nothing truthful to report, so the session is reported as free rather than as held
			// by a placeholder name.
			Expect(currentOf()).To(BeNil())
		}))

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

		Expect(connected).To(BeTrue())
	})

	It("answers who holds the session without touching it", func() {
		_, err := manager.Acquire(asUser("alice"), vm, subv1alpha2.ConsoleSession, "d8/v1.2.3")
		Expect(err).NotTo(HaveOccurred())
		held := leaseOf()

		base := &BaseREST{sessions: manager}
		w := httptest.NewRecorder()
		base.probeSession(vm, subv1alpha2.ConsoleSession).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?probe=true", nil).WithContext(asUser("bob")))

		Expect(w.Code).To(Equal(http.StatusOK))
		var answer subv1alpha2.VirtualMachineSession
		Expect(json.Unmarshal(w.Body.Bytes(), &answer)).To(Succeed())
		Expect(answer.Holder).To(Equal("alice"))
		Expect(answer.Client).To(Equal("d8/v1.2.3"))
		Expect(answer.StartTime).NotTo(BeNil())

		// Probing is read-only: alice keeps the very lease she had.
		Expect(leaseOf().ResourceVersion).To(Equal(held.ResourceVersion))
		Expect(holderOf()).To(Equal("alice"))
	})

	It("answers that a session nobody holds is free", func() {
		base := &BaseREST{sessions: manager}
		w := httptest.NewRecorder()
		base.probeSession(vm, subv1alpha2.VNCSession).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?probe=true", nil).WithContext(asUser("bob")))

		Expect(w.Code).To(Equal(http.StatusOK))
		var answer subv1alpha2.VirtualMachineSession
		Expect(json.Unmarshal(w.Body.Bytes(), &answer)).To(Succeed())
		Expect(answer.Holder).To(BeEmpty())
		Expect(answer.StartTime).To(BeNil())
	})

	It("connects even when the session cannot be recorded", func() {
		kubeCli.PrependReactor("*", "leases", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("leases are unavailable")
		})

		connected := false
		base := &BaseREST{sessions: manager}
		handler := base.trackSession(vm, subv1alpha2.ConsoleSession, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			connected = true
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(asUser("alice")))

		Expect(connected).To(BeTrue())
	})
})
