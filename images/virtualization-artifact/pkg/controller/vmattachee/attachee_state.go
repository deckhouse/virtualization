/*
Copyright 2024 Flant JSC

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

package vmattachee

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StateReconciler interface {
	two_phase_reconciler.ReconcilerState
	AttacheeChecker
}

type AttacheeChecker interface {
	IsAttachedToVM(vm virtv2.VirtualMachine) bool
}

type AttacheeState[T helper.Object[T, ST], ST any] struct {
	StateReconciler

	isAttached bool

	ProtectionFinalizer string
	Resource            *helper.Resource[T, ST]
}

func NewAttacheeState[T helper.Object[T, ST], ST any](
	reconcilerState StateReconciler,
	protectionFinalizer string,
	resource *helper.Resource[T, ST],
) *AttacheeState[T, ST] {
	return &AttacheeState[T, ST]{
		StateReconciler:     reconcilerState,
		ProtectionFinalizer: protectionFinalizer,
		Resource:            resource,
	}
}

func (state *AttacheeState[T, ST]) ShouldReconcile(log logr.Logger) bool {
	log.V(2).Info("AttacheeState ShouldReconcile", "ShouldRemoveProtectionFinalizer", state.ShouldRemoveProtectionFinalizer())
	return state.ShouldRemoveProtectionFinalizer()
}

func (state *AttacheeState[T, ST]) Reload(ctx context.Context, _ reconcile.Request, log logr.Logger, client client.Client) error {
	isAttached, err := state.hasAttachedVM(ctx, client)
	if err != nil {
		return err
	}
	state.isAttached = isAttached

	log.V(2).Info("Attachee Reload", "isAttached", state.isAttached)

	return nil
}

func (state *AttacheeState[T, ST]) hasAttachedVM(ctx context.Context, apiClient client.Client) (bool, error) {
	var vms virtv2.VirtualMachineList
	err := apiClient.List(ctx, &vms, &client.ListOptions{
		Namespace: state.Resource.Name().Namespace,
	})
	if err != nil {
		return false, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		if state.IsAttachedToVM(vm) {
			return true, nil
		}
	}

	return false, nil
}

func (state *AttacheeState[T, ST]) ShouldRemoveProtectionFinalizer() bool {
	return controllerutil.ContainsFinalizer(state.Resource.Current(), state.ProtectionFinalizer) && !state.isAttached
}

func (state *AttacheeState[T, ST]) RemoveProtectionFinalizer() {
	controllerutil.RemoveFinalizer(state.Resource.Changed(), state.ProtectionFinalizer)
}
