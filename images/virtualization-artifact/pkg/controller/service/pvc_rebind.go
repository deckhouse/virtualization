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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

// rebindOriginalReclaimPolicyAnnotation stores the PersistentVolume reclaim
// policy that was in effect before Rebind temporarily switched it to Retain, so
// it can be restored once the target PVC is bound.
const rebindOriginalReclaimPolicyAnnotation = "virtualization.deckhouse.io/rebind-original-reclaim-policy"

// Rebind transfers the PersistentVolume that backs the prime PersistentVolumeClaim
// to the target PersistentVolumeClaim, following the CDI volume-populator pattern,
// and then removes the prime PVC. This lets an importer populate a dedicated prime
// PVC while the target PVC (the one a VirtualMachine consumes) stays untouched, so
// the importer pod and the VM never contend for the same ReadWriteOnce volume.
//
// The operation is idempotent and resumable: call it repeatedly until it returns
// true. The steps are ordered so the populated PV is never lost, even if the
// process crashes between calls:
//
//  1. the PV reclaim policy is switched to Retain (original saved in an annotation);
//  2. the PV ClaimRef is pointed at the target PVC;
//  3. the target PVC VolumeName is pointed at the PV;
//  4. the prime PVC is deleted (safe: the PV now belongs to the target and is Retained);
//  5. once the target PVC is Bound, the original reclaim policy is restored.
//
// Binding itself is completed by the cluster's PersistentVolume binder; Rebind
// returns false until the target PVC reaches the Bound phase.
func Rebind(ctx context.Context, c client.Client, prime, target types.NamespacedName) (bool, error) {
	targetPVC, err := object.FetchObject(ctx, target, c, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, fmt.Errorf("fetch target pvc %s: %w", target, err)
	}
	if targetPVC == nil {
		return false, fmt.Errorf("target pvc %s not found", target)
	}

	primePVC, err := object.FetchObject(ctx, prime, c, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, fmt.Errorf("fetch prime pvc %s: %w", prime, err)
	}

	// Resolve the PV to transfer: prefer the one the target already points at
	// (resumed run), otherwise the one bound to the prime PVC.
	pvName := targetPVC.Spec.VolumeName
	if pvName == "" && primePVC != nil {
		pvName = primePVC.Spec.VolumeName
	}
	if pvName == "" {
		if primePVC == nil {
			return false, fmt.Errorf("cannot rebind: prime pvc %s is gone and target %s is not bound to any volume", prime, target)
		}
		// Prime PVC exists but has not been provisioned/bound yet; wait.
		return false, nil
	}

	pv, err := object.FetchObject(ctx, types.NamespacedName{Name: pvName}, c, &corev1.PersistentVolume{})
	if err != nil {
		return false, fmt.Errorf("fetch persistent volume %q: %w", pvName, err)
	}
	if pv == nil {
		return false, fmt.Errorf("persistent volume %q not found", pvName)
	}

	// Step 1: Retain the PV so deleting the prime PVC can never delete the volume.
	if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		if pv.Annotations == nil {
			pv.Annotations = map[string]string{}
		}
		if _, ok := pv.Annotations[rebindOriginalReclaimPolicyAnnotation]; !ok {
			pv.Annotations[rebindOriginalReclaimPolicyAnnotation] = string(pv.Spec.PersistentVolumeReclaimPolicy)
		}
		pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
		if err := c.Update(ctx, pv); err != nil {
			return false, fmt.Errorf("retain persistent volume %q: %w", pvName, err)
		}
	}

	// Step 2: Point the PV at the target PVC.
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID != targetPVC.UID {
		pv.Spec.ClaimRef = &corev1.ObjectReference{
			Kind:            "PersistentVolumeClaim",
			APIVersion:      "v1",
			Namespace:       targetPVC.Namespace,
			Name:            targetPVC.Name,
			UID:             targetPVC.UID,
			ResourceVersion: targetPVC.ResourceVersion,
		}
		if err := c.Update(ctx, pv); err != nil {
			return false, fmt.Errorf("rebind persistent volume %q claimRef to target: %w", pvName, err)
		}
	}

	// Step 3: Point the target PVC at the PV.
	if targetPVC.Spec.VolumeName != pvName {
		targetPVC.Spec.VolumeName = pvName
		if err := c.Update(ctx, targetPVC); err != nil {
			return false, fmt.Errorf("set target pvc %s volumeName: %w", target, err)
		}
	}

	// Step 4: Delete the prime PVC. The PV is safe now: its ClaimRef points to
	// the target and its reclaim policy is Retain.
	if primePVC != nil && primePVC.DeletionTimestamp == nil {
		if err := c.Delete(ctx, primePVC); err != nil && !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("delete prime pvc %s: %w", prime, err)
		}
	}

	// Step 5: The PV binder finalizes the bind. Once the target is Bound, restore
	// the original reclaim policy and report completion.
	if targetPVC.Status.Phase != corev1.ClaimBound {
		return false, nil
	}

	if err := restoreReclaimPolicy(ctx, c, pv); err != nil {
		return false, err
	}
	return true, nil
}

// restoreReclaimPolicy reverts the PV reclaim policy to the value saved before
// Rebind switched it to Retain. It is a no-op when no original value was saved.
func restoreReclaimPolicy(ctx context.Context, c client.Client, pv *corev1.PersistentVolume) error {
	original, ok := pv.Annotations[rebindOriginalReclaimPolicyAnnotation]
	if !ok || original == "" || original == string(corev1.PersistentVolumeReclaimRetain) {
		// Nothing to restore (it was Retain to begin with, or already restored).
		if ok {
			delete(pv.Annotations, rebindOriginalReclaimPolicyAnnotation)
			return c.Update(ctx, pv)
		}
		return nil
	}

	pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimPolicy(original)
	delete(pv.Annotations, rebindOriginalReclaimPolicyAnnotation)
	if err := c.Update(ctx, pv); err != nil {
		return fmt.Errorf("restore persistent volume %q reclaim policy: %w", pv.Name, err)
	}
	return nil
}
