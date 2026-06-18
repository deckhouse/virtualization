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
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	rebindNS       = "ns"
	rebindPrimeNm  = "prime"
	rebindTargetNm = "target"
	rebindPVNm     = "pv-1"
)

func rebindClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(rebindScheme()).
		WithStatusSubresource(&corev1.PersistentVolumeClaim{}, &corev1.PersistentVolume{}).
		WithObjects(objs...).
		Build()
}

func rebindScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func newBoundPVC(name string, uid types.UID) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rebindNS, UID: uid},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: rebindPVNm},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}

func newPendingPVC(name string, uid types.UID) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rebindNS, UID: uid},
		Spec:       corev1.PersistentVolumeClaimSpec{},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}
}

func newPV(claimRefName string, claimRefUID types.UID, reclaim corev1.PersistentVolumeReclaimPolicy) *corev1.PersistentVolume {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: rebindPVNm},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: reclaim,
		},
	}
	if claimRefName != "" {
		pv.Spec.ClaimRef = &corev1.ObjectReference{
			Kind:      "PersistentVolumeClaim",
			Namespace: rebindNS,
			Name:      claimRefName,
			UID:       claimRefUID,
		}
	}
	return pv
}

func getPVC(t *testing.T, c client.Client, name string) *corev1.PersistentVolumeClaim {
	t.Helper()
	pvc := &corev1.PersistentVolumeClaim{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: rebindNS, Name: name}, pvc)
	if err != nil {
		t.Fatalf("get pvc %s: %v", name, err)
	}
	return pvc
}

func getPV(t *testing.T, c client.Client, name string) *corev1.PersistentVolume {
	t.Helper()
	pv := &corev1.PersistentVolume{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: name}, pv); err != nil {
		t.Fatalf("get pv %s: %v", name, err)
	}
	return pv
}

func primeKey() types.NamespacedName {
	return types.NamespacedName{Namespace: rebindNS, Name: rebindPrimeNm}
}

func targetKey() types.NamespacedName {
	return types.NamespacedName{Namespace: rebindNS, Name: rebindTargetNm}
}

// TestRebindTransfersPVAndDeletesPrime covers the first (not-yet-bound) pass:
// the PV is retained and rebound to the target, the target points at the PV, and
// the prime PVC is deleted. Binding is finalized by the cluster binder afterwards,
// so Rebind returns false.
func TestRebindTransfersPVAndDeletesPrime(t *testing.T) {
	prime := newBoundPVC(rebindPrimeNm, "prime-uid")
	target := newPendingPVC(rebindTargetNm, "target-uid")
	pv := newPV(rebindPrimeNm, "prime-uid", corev1.PersistentVolumeReclaimDelete)
	c := rebindClient(prime, target, pv)

	done, err := Rebind(context.Background(), c, primeKey(), targetKey())
	if err != nil {
		t.Fatalf("Rebind returned error: %v", err)
	}
	if done {
		t.Fatalf("expected done=false while target is not yet Bound")
	}

	gotPV := getPV(t, c, rebindPVNm)
	if gotPV.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		t.Errorf("expected PV reclaim policy Retain, got %q", gotPV.Spec.PersistentVolumeReclaimPolicy)
	}
	if got := gotPV.Annotations[rebindOriginalReclaimPolicyAnnotation]; got != string(corev1.PersistentVolumeReclaimDelete) {
		t.Errorf("expected saved original reclaim policy Delete, got %q", got)
	}
	if gotPV.Spec.ClaimRef == nil || gotPV.Spec.ClaimRef.Name != rebindTargetNm || gotPV.Spec.ClaimRef.UID != "target-uid" {
		t.Errorf("expected PV claimRef pointing to target, got %#v", gotPV.Spec.ClaimRef)
	}

	gotTarget := getPVC(t, c, rebindTargetNm)
	if gotTarget.Spec.VolumeName != rebindPVNm {
		t.Errorf("expected target volumeName %q, got %q", rebindPVNm, gotTarget.Spec.VolumeName)
	}

	if err := c.Get(context.Background(), primeKey(), &corev1.PersistentVolumeClaim{}); !k8serrors.IsNotFound(err) {
		t.Errorf("expected prime PVC to be deleted, got err=%v", err)
	}
}

// TestRebindCompletesWhenTargetBound covers the final pass: once the binder has
// bound the target, Rebind restores the original reclaim policy and reports done.
func TestRebindCompletesWhenTargetBound(t *testing.T) {
	// Target already bound to the PV (binder finished).
	target := newBoundPVC(rebindTargetNm, "target-uid")
	pv := newPV(rebindTargetNm, "target-uid", corev1.PersistentVolumeReclaimRetain)
	pv.Annotations = map[string]string{rebindOriginalReclaimPolicyAnnotation: string(corev1.PersistentVolumeReclaimDelete)}
	// Prime already gone.
	c := rebindClient(target, pv)

	done, err := Rebind(context.Background(), c, primeKey(), targetKey())
	if err != nil {
		t.Fatalf("Rebind returned error: %v", err)
	}
	if !done {
		t.Fatalf("expected done=true when target is Bound")
	}

	gotPV := getPV(t, c, rebindPVNm)
	if gotPV.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimDelete {
		t.Errorf("expected reclaim policy restored to Delete, got %q", gotPV.Spec.PersistentVolumeReclaimPolicy)
	}
	if _, ok := gotPV.Annotations[rebindOriginalReclaimPolicyAnnotation]; ok {
		t.Errorf("expected original-reclaim-policy annotation to be removed")
	}
}

// TestRebindEndToEnd drives the full lifecycle through repeated calls, simulating
// the PV binder between calls, and asserts idempotency.
func TestRebindEndToEnd(t *testing.T) {
	prime := newBoundPVC(rebindPrimeNm, "prime-uid")
	target := newPendingPVC(rebindTargetNm, "target-uid")
	pv := newPV(rebindPrimeNm, "prime-uid", corev1.PersistentVolumeReclaimDelete)
	c := rebindClient(prime, target, pv)
	ctx := context.Background()

	// Pass 1: rebind spec changes, prime deleted, not yet bound.
	done, err := Rebind(ctx, c, primeKey(), targetKey())
	if err != nil || done {
		t.Fatalf("pass 1: done=%v err=%v, want false/nil", done, err)
	}

	// Pass 2 (idempotent re-run before the binder acts): still not bound, no error.
	done, err = Rebind(ctx, c, primeKey(), targetKey())
	if err != nil || done {
		t.Fatalf("pass 2 (idempotent): done=%v err=%v, want false/nil", done, err)
	}

	// Simulate the cluster PV binder finishing the bind (status subresource).
	gotTarget := getPVC(t, c, rebindTargetNm)
	gotTarget.Status.Phase = corev1.ClaimBound
	if err := c.Status().Update(ctx, gotTarget); err != nil {
		t.Fatalf("simulate binder: %v", err)
	}

	// Pass 3: target Bound -> completion, reclaim policy restored.
	done, err = Rebind(ctx, c, primeKey(), targetKey())
	if err != nil {
		t.Fatalf("pass 3: err=%v", err)
	}
	if !done {
		t.Fatalf("pass 3: expected done=true after target Bound")
	}

	gotPV := getPV(t, c, rebindPVNm)
	if gotPV.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimDelete {
		t.Errorf("expected reclaim policy restored to Delete, got %q", gotPV.Spec.PersistentVolumeReclaimPolicy)
	}
	if gotPV.Spec.ClaimRef == nil || gotPV.Spec.ClaimRef.UID != "target-uid" {
		t.Errorf("expected PV claimRef to target, got %#v", gotPV.Spec.ClaimRef)
	}

	// Pass 4: fully idempotent once done.
	done, err = Rebind(ctx, c, primeKey(), targetKey())
	if err != nil || !done {
		t.Fatalf("pass 4 (idempotent done): done=%v err=%v, want true/nil", done, err)
	}
}

// TestRebindAlreadyDone returns true immediately when the target is already bound
// and the prime PVC no longer exists.
func TestRebindAlreadyDone(t *testing.T) {
	target := newBoundPVC(rebindTargetNm, "target-uid")
	pv := newPV(rebindTargetNm, "target-uid", corev1.PersistentVolumeReclaimDelete)
	c := rebindClient(target, pv)

	done, err := Rebind(context.Background(), c, primeKey(), targetKey())
	if err != nil || !done {
		t.Fatalf("done=%v err=%v, want true/nil", done, err)
	}
}

// TestRebindWaitsWhenPrimeNotBound returns (false, nil) without touching anything
// while the prime PVC has not been provisioned yet.
func TestRebindWaitsWhenPrimeNotBound(t *testing.T) {
	prime := newPendingPVC(rebindPrimeNm, "prime-uid") // no VolumeName yet
	target := newPendingPVC(rebindTargetNm, "target-uid")
	c := rebindClient(prime, target)

	done, err := Rebind(context.Background(), c, primeKey(), targetKey())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatalf("expected done=false while prime is not bound")
	}
	// Prime must still exist.
	if err := c.Get(context.Background(), primeKey(), &corev1.PersistentVolumeClaim{}); err != nil {
		t.Errorf("prime PVC must not be deleted while unbound: %v", err)
	}
}

func TestRebindErrors(t *testing.T) {
	t.Run("target not found", func(t *testing.T) {
		c := rebindClient(newBoundPVC(rebindPrimeNm, "prime-uid"))
		if _, err := Rebind(context.Background(), c, primeKey(), targetKey()); err == nil {
			t.Fatal("expected error when target PVC is missing")
		}
	})

	t.Run("prime gone and target not bound", func(t *testing.T) {
		c := rebindClient(newPendingPVC(rebindTargetNm, "target-uid"))
		if _, err := Rebind(context.Background(), c, primeKey(), targetKey()); err == nil {
			t.Fatal("expected error when prime is gone and target is unbound")
		}
	})

	t.Run("pv not found", func(t *testing.T) {
		c := rebindClient(
			newBoundPVC(rebindPrimeNm, "prime-uid"),
			newPendingPVC(rebindTargetNm, "target-uid"),
		)
		if _, err := Rebind(context.Background(), c, primeKey(), targetKey()); err == nil {
			t.Fatal("expected error when the backing PV is missing")
		}
	})
}
