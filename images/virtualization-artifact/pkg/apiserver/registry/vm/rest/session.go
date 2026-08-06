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
	"errors"
	"net/http"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/apiserver/pkg/endpoints/request"
	coordinationclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const (
	// sessionLeaseDuration is how long a session lease stays valid without a renewal.
	// After it expires the session is reported as free, so a replica that died mid-stream
	// cannot leave the console of a virtual machine marked as busy forever.
	sessionLeaseDuration  = 30 * time.Second
	sessionReleaseTimeout = 5 * time.Second

	sessionIDAnnotation     = "virtualization.deckhouse.io/session-id"
	sessionClientAnnotation = "virtualization.deckhouse.io/session-client"
	sessionKindLabel        = "virtualization.deckhouse.io/session"

	// maxSessionClientLen bounds what a client can write about itself into the lease.
	maxSessionClientLen = 128
)

// sessionRenewInterval is how often a held session is renewed. A variable so that a test does
// not have to wait for it.
var sessionRenewInterval = 10 * time.Second

// sessionManager tracks who currently streams the serial console or the VNC of a virtual
// machine. Both streams are exclusive: connecting disconnects whoever was there before.
// That behaviour is kept as is — the manager only makes it visible, so a client can ask who
// is going to be disconnected and warn its user before connecting.
//
// The state lives in a coordination.k8s.io Lease in the namespace of the virtual machine
// because virtualization-api runs in several replicas and an in-memory map would report a
// session held by another replica as free. The lease cannot get stuck in a wrong state:
// it is renewed while the stream is open and expires on its own once renewals stop (replica
// crash, network partition, killed pod), and it is owned by the virtual machine, so it is
// garbage collected together with it.
//
// Tracking is advisory: it never blocks a connection. Any failure to write the lease is
// logged and the stream proceeds, at worst leaving the session reported as free.
type sessionManager struct {
	leases   coordinationclient.LeasesGetter
	recorder record.EventRecorder
}

func newSessionManager(leases coordinationclient.LeasesGetter, recorder record.EventRecorder) *sessionManager {
	return &sessionManager{leases: leases, recorder: recorder}
}

// session is an acquired lease held for the lifetime of a single stream.
type session struct {
	manager *sessionManager
	id      string
	kind    subv1alpha2.SessionKind

	mu    sync.Mutex
	lease *coordinationv1.Lease
	// lost is set once the lease is taken over by somebody else: the stream is already
	// broken at that point and the lease must not be renewed or released anymore.
	lost bool
}

// sessionInfo describes a session that is currently being held.
type sessionInfo struct {
	Holder    string
	StartTime *metav1.Time
	// Client is the user agent of the connected client, as it introduced itself. It tells a
	// UI from a CLI, which is often what one needs to go and find the person.
	Client string
}

// Current reports the session of the virtual machine that is being held right now, or nil when
// nobody is connected.
func (m *sessionManager) Current(ctx context.Context, vm *v1alpha2.VirtualMachine, kind subv1alpha2.SessionKind) (*sessionInfo, error) {
	lease, err := m.leases.Leases(vm.Namespace).Get(ctx, sessionLeaseName(vm, kind), metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	holder := activeHolder(lease)
	if holder == "" {
		return nil, nil
	}
	info := &sessionInfo{Holder: holder, Client: lease.Annotations[sessionClientAnnotation]}
	if lease.Spec.AcquireTime != nil {
		info.StartTime = ptr.To(metav1.NewTime(lease.Spec.AcquireTime.Time))
	}
	return info, nil
}

// Acquire records the user of the request as the holder of the session. It always succeeds in
// taking the session over, mirroring what the virtual machine does with the stream itself;
// displacing somebody else is reported as an event on the virtual machine.
func (m *sessionManager) Acquire(ctx context.Context, vm *v1alpha2.VirtualMachine, kind subv1alpha2.SessionKind, client string) (*session, error) {
	user := requestUserName(ctx)
	if user == "" {
		// Nothing truthful to report as the holder. A placeholder name in the dialog of the next
		// user would be worse than no answer: tracking is advisory and the stream proceeds anyway.
		return nil, errors.New("the request carries no authenticated user")
	}
	name := sessionLeaseName(vm, kind)
	leases := m.leases.Leases(vm.Namespace)

	s := &session{manager: m, id: string(uuid.NewUUID()), kind: kind}
	var displaced string

	// Two clients connecting at the same instant race for the lease; the loser retries and
	// ends up as the holder, which matches the stream it has just taken over.
	err := retry.OnError(retry.DefaultRetry, isRaceError, func() error {
		current, err := leases.Get(ctx, name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			created, createErr := leases.Create(ctx, s.newLease(vm, name, user, client), metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
			displaced, s.lease = "", created
			return nil
		}
		if err != nil {
			return err
		}

		taken := s.newLease(vm, name, user, client)
		taken.ResourceVersion = current.ResourceVersion
		taken.Spec.LeaseTransitions = nextTransition(current)
		updated, err := leases.Update(ctx, taken, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		if holder := activeHolder(current); holder != user {
			displaced = holder
		}
		s.lease = updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	if displaced != "" {
		m.recorder.Eventf(vm, corev1.EventTypeWarning, sessionPreemptedReason(kind),
			"The %s of the VirtualMachine was taken over by user %q, the session of user %q was disconnected.",
			kind.Description(), user, displaced)
	}
	return s, nil
}

func (s *session) newLease(vm *v1alpha2.VirtualMachine, name, user, client string) *coordinationv1.Lease {
	now := metav1.NewMicroTime(time.Now())
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: vm.Namespace,
			Annotations: map[string]string{
				sessionIDAnnotation:     s.id,
				sessionClientAnnotation: client,
			},
			// Marks the lease as ours among all the leases of a cluster, so session leases can
			// be selected without knowing their names.
			Labels: map[string]string{sessionKindLabel: string(s.kind)},
			// The lease never outlives the virtual machine it describes.
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
				Name:       vm.Name,
				UID:        vm.UID,
			}},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(user),
			LeaseDurationSeconds: ptr.To(int32(sessionLeaseDuration.Seconds())),
			AcquireTime:          &now,
			RenewTime:            &now,
		},
	}
}

// keep holds the session for as long as the request runs and returns the release, to be
// deferred by the caller.
//
// Both the renewal and the release run on a detached copy of the request context. A stream
// subresource is not one of those the apiserver treats as long running, so the request carries
// a deadline of its own; the deadline does not end the stream, which is served over a hijacked
// connection, and a session whose renewals stopped there would be reported as free while
// somebody is still connected. What ends the renewals is the release, called when the stream is
// over.
func (s *session) keep(r *http.Request) func() {
	detached := context.WithoutCancel(r.Context())
	renewCtx, stopRenewing := context.WithCancel(detached)
	releaseCtx := detached
	go s.keepAlive(renewCtx)
	return func() {
		stopRenewing()
		s.release(releaseCtx)
	}
}

func (s *session) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(sessionRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.renew(ctx); err != nil {
				// A failed renewal is not fatal: the next tick retries, and if none of them
				// succeeds the lease expires, which is exactly what an unreachable session
				// should look like.
				klog.FromContext(ctx).Error(err, "failed to renew the session lease")
			}
		}
	}
}

func (s *session) renew(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lost {
		return nil
	}

	leases := s.manager.leases.Leases(s.lease.Namespace)
	current, err := leases.Get(ctx, s.lease.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			s.lost = true
			return nil
		}
		return err
	}
	if current.Annotations[sessionIDAnnotation] != s.id {
		s.lost = true
		return nil
	}

	now := metav1.NewMicroTime(time.Now())
	current.Spec.RenewTime = &now
	renewed, err := leases.Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	s.lease = renewed
	return nil
}

func (s *session) release(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lost {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, sessionReleaseTimeout)
	defer cancel()

	// Delete only our own lease: somebody may have taken the session over between the last
	// renewal and now. Leaving a stale lease behind is harmless, it expires by itself.
	err := s.manager.leases.Leases(s.lease.Namespace).Delete(ctx, s.lease.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &s.lease.UID, ResourceVersion: &s.lease.ResourceVersion},
	})
	if err != nil && !k8serrors.IsNotFound(err) && !k8serrors.IsConflict(err) {
		klog.Background().Error(err, "failed to release the session lease", "lease", s.lease.Name)
	}
}

// trackSession records the session and keeps it recorded while the handler streams. It records
// on the way in, not when the handler is built, so that a connection that is never established
// leaves nothing behind and so that the client can be taken from the request itself.
//
// Tracking is best effort: it must never keep a user away from the console of a virtual
// machine, so a failure is logged and the stream proceeds untracked.
func (r *BaseREST) trackSession(vm *v1alpha2.VirtualMachine, kind subv1alpha2.SessionKind, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		session, err := r.sessions.Acquire(ctx, vm, kind, sessionClient(req))
		if err != nil {
			klog.FromContext(ctx).Error(err, "failed to record the session, connecting without it",
				"virtualMachine", vm.Name, "namespace", vm.Namespace, "session", kind)
		} else {
			release := session.keep(req)
			defer release()
		}

		handler.ServeHTTP(w, req)
	})
}

// probeSession answers who holds the session instead of connecting to it, leaving the holder
// alone. It is a mode of the stream itself rather than a handle of its own: a client asks the
// very endpoint it is about to connect to, and a client that does not ask connects exactly as
// it did before.
func (r *BaseREST) probeSession(vm *v1alpha2.VirtualMachine, kind subv1alpha2.SessionKind) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		current, err := r.sessions.Current(req.Context(), vm, kind)
		if err != nil {
			status := k8serrors.NewInternalError(err).Status()
			responsewriters.WriteRawJSON(int(status.Code), status, w)
			return
		}

		// Written into the not-yet-upgraded connection as it is: this is a stream endpoint, so the
		// answer goes through neither the scheme nor the conversion of a regular resource.
		result := &subv1alpha2.VirtualMachineSession{}
		if current != nil {
			result.Holder = current.Holder
			result.StartTime = current.StartTime
			result.Client = current.Client
		}
		responsewriters.WriteRawJSON(http.StatusOK, result, w)
	})
}

// sessionClient is how the connected client introduced itself. Bounded, because it is written
// into an object on behalf of whoever sent the header, and cut by runes rather than by bytes so
// that a multi-byte character on the boundary does not become a broken annotation value.
func sessionClient(req *http.Request) string {
	agent := []rune(req.UserAgent())
	if len(agent) > maxSessionClientLen {
		agent = agent[:maxSessionClientLen]
	}
	return string(agent)
}

// activeHolder returns the user of a session that is still being renewed, or an empty string
// if the lease has expired and the session is free.
func activeHolder(lease *coordinationv1.Lease) string {
	if lease.Spec.HolderIdentity == nil || lease.Spec.RenewTime == nil {
		return ""
	}
	duration := sessionLeaseDuration
	if lease.Spec.LeaseDurationSeconds != nil {
		duration = time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	}
	if time.Since(lease.Spec.RenewTime.Time) > duration {
		return ""
	}
	return *lease.Spec.HolderIdentity
}

// sessionPreemptedReason is the reason of the event reporting a taken over session. Every kind is
// listed on purpose: a kind added later shows up here instead of being reported as a console.
func sessionPreemptedReason(kind subv1alpha2.SessionKind) string {
	switch kind {
	case subv1alpha2.ConsoleSession:
		return "ConsoleSessionPreempted"
	case subv1alpha2.VNCSession:
		return "VNCSessionPreempted"
	default:
		return "SessionPreempted"
	}
}

// sessionLeaseName follows the naming of underlying resources: d8v-vm-<kind>-<name>-<uid>.
func sessionLeaseName(vm *v1alpha2.VirtualMachine, kind subv1alpha2.SessionKind) string {
	return supplements.NewGenerator("vm-"+string(kind), vm.Name, vm.Namespace, vm.UID).CommonSupplement().Name
}

func nextTransition(lease *coordinationv1.Lease) *int32 {
	transitions := int32(0)
	if lease.Spec.LeaseTransitions != nil {
		transitions = *lease.Spec.LeaseTransitions
	}
	return ptr.To(transitions + 1)
}

func isRaceError(err error) bool {
	return k8serrors.IsConflict(err) || k8serrors.IsAlreadyExists(err) || k8serrors.IsNotFound(err)
}

func requestUserName(ctx context.Context) string {
	if user, ok := request.UserFrom(ctx); ok {
		return user.GetName()
	}
	return ""
}
