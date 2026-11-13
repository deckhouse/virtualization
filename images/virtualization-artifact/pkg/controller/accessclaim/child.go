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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ParentRef returns the VirtualMachineSnapshot a child snapshot is planned by, or nil for a snapshot taken
// on its own. A child is captured under the access of its parent and has no claim of its own.
func ParentRef(vds *v1alpha2.VirtualDiskSnapshot) *metav1.OwnerReference {
	for i, ref := range vds.OwnerReferences {
		if ref.Kind == v1alpha2.VirtualMachineSnapshotKind && ref.APIVersion == v1alpha2.SchemeGroupVersion.String() {
			return &vds.OwnerReferences[i]
		}
	}

	return nil
}

// ChildConsistency returns the consistency a child snapshot takes from its parent, whose session covers the
// capture of every child, and settled=false while the parent is not terminal. A child whose parent is gone or
// recreated is not consistent.
func ChildConsistency(ctx context.Context, c client.Reader, vds *v1alpha2.VirtualDiskSnapshot, parent *metav1.OwnerReference) (consistent, settled bool, err error) {
	vms := &v1alpha2.VirtualMachineSnapshot{}

	err = c.Get(ctx, types.NamespacedName{Namespace: vds.Namespace, Name: parent.Name}, vms)
	switch {
	case apierrors.IsNotFound(err):
		return false, true, nil
	case err != nil:
		return false, false, err
	case vms.UID != parent.UID:
		return false, true, nil
	}

	switch vms.Status.Phase {
	case v1alpha2.VirtualMachineSnapshotPhaseReady, v1alpha2.VirtualMachineSnapshotPhaseFailed:
		return ptr.Deref(vms.Status.Consistent, false), true, nil
	default:
		return false, false, nil
	}
}
