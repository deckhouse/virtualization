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

package internal

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ProtectionHandler adds the coordinator's finalizer to every claim that has yet to be decided, so a
// claim deleted while it holds the virtual machine is not gone before the guest is unfrozen.
type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h *ProtectionHandler) Name() string {
	return "ProtectionHandler"
}

func (h *ProtectionHandler) Handle(_ context.Context, s *state.State) (reconcile.Result, error) {
	for _, c := range s.Claims {
		if c.Changed.DeletionTimestamp == nil && !isTerminal(c.Changed) {
			controllerutil.AddFinalizer(c.Changed, v1alpha2.FinalizerVMACCleanup)
		}
	}

	return reconcile.Result{}, nil
}

// CleanupHandler removes the finalizer from every claim that no longer holds the virtual machine and is
// decided or being deleted. It runs after LifecycleHandler, which releases the lock first.
type CleanupHandler struct{}

func NewCleanupHandler() *CleanupHandler {
	return &CleanupHandler{}
}

func (h *CleanupHandler) Name() string {
	return "CleanupHandler"
}

func (h *CleanupHandler) Handle(_ context.Context, s *state.State) (reconcile.Result, error) {
	holder := s.Holder()
	for _, c := range s.Claims {
		if c == holder {
			continue
		}

		if c.Changed.DeletionTimestamp != nil || isTerminal(c.Changed) {
			controllerutil.RemoveFinalizer(c.Changed, v1alpha2.FinalizerVMACCleanup)
		}
	}

	return reconcile.Result{}, nil
}
