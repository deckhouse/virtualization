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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements/copier"
)

// modulePullSecretRefs returns imagePullSecrets for a provisioner pod. The pod
// references the per-owner copy of the module registry secret (see
// ensureModulePullSecret), not the source secret in the module namespace. An
// empty source disables the reference: it is only needed when the module
// registry differs from the cluster registry, so node-level containerd auth
// cannot pull the module image.
func modulePullSecretRefs(src types.NamespacedName, sup supplements.Generator) []corev1.LocalObjectReference {
	if src.Name == "" {
		return nil
	}
	return []corev1.LocalObjectReference{{Name: sup.ModuleRegistrySecret().Name}}
}

// ensureModulePullSecret copies the module registry pull secret into the
// namespace of provisioner pods under the per-owner supplemental name and
// refreshes the copy on every reconcile so it follows source rotation.
func ensureModulePullSecret(ctx context.Context, c client.Client, src types.NamespacedName, sup supplements.Generator, ownerRef metav1.OwnerReference) error {
	if src.Name == "" {
		return nil
	}
	// An owner pod that lost the create race carries no UID yet; skip and let
	// the next reconcile attach the fetched pod as the owner.
	if ownerRef.UID == "" {
		return nil
	}
	// The secret must never block owner pod finalization: under foreground
	// cascading deletion a copy shared with a still-living second pod would
	// wedge the first pod in Terminating.
	ownerRef.BlockOwnerDeletion = ptr.To(false)

	secretCopier := copier.Secret{
		Source:         src,
		Destination:    sup.ModuleRegistrySecret(),
		OwnerReference: ownerRef,
	}
	if err := secretCopier.CopyOrUpdate(ctx, c); err != nil {
		return fmt.Errorf("copy module registry pull secret: %w", err)
	}
	return nil
}
