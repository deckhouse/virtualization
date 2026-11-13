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

package accessclaim

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
)

func newClient(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := coordinationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
}

func claim() *coordinationv1alpha1.VirtualMachineAccessClaim {
	return &coordinationv1alpha1.VirtualMachineAccessClaim{ObjectMeta: metav1.ObjectMeta{Name: "vms-uid", Namespace: "ns"}}
}

// A claim created moments ago may be missing from the cache; the API server still has it.
func TestGetFallsBackToTheAPIServerOnACacheMiss(t *testing.T) {
	cache := newClient(t, interceptor.Funcs{})
	api := newClient(t, interceptor.Funcs{}, claim())

	got, err := Get(context.Background(), cache, api, "ns", "vms-uid")
	if err != nil {
		t.Fatal(err)
	}

	if got == nil {
		t.Fatal("the claim missing from the cache was not read from the API server")
	}
}

// A claim the cache holds is not read from the API server.
func TestGetReadsACachedClaimFromTheCacheOnly(t *testing.T) {
	cache := newClient(t, interceptor.Funcs{}, claim())
	api := newClient(t, interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errors.New("the API server was read")
		},
	})

	got, err := Get(context.Background(), cache, api, "ns", "vms-uid")
	if err != nil {
		t.Fatal(err)
	}

	if got == nil {
		t.Fatal("the cached claim was not returned")
	}
}

// A claim neither the cache nor the API server holds does not exist.
func TestGetReturnsNilForAMissingClaim(t *testing.T) {
	got, err := Get(context.Background(), newClient(t, interceptor.Funcs{}), newClient(t, interceptor.Funcs{}), "ns", "vms-uid")
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

// A session granted on a stopped machine completes consistent; a claim released before it was granted
// completes the same way but had no session, so it is not.
func TestConsistentRequiresAGrant(t *testing.T) {
	stopped := claim()
	stopped.Status.Phase = coordinationv1alpha1.AccessClaimPhaseCompleted

	stopped.Status.GrantedAt = &metav1.Time{}
	if !Consistent(stopped) {
		t.Error("a session granted on a stopped machine is not consistent")
	}

	released := claim()

	released.Status.Phase = coordinationv1alpha1.AccessClaimPhaseCompleted
	if Consistent(released) {
		t.Error("a claim released before it was granted is consistent")
	}
}
