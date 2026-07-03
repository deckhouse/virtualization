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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/retry"

	virtclient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

// ScaleDownWithREST serves the addressed scale-down handle:
//
//	POST .../virtualmachinepools/<name>/scaledownwith  {"targets": [...]}
//
// It validates that every target belongs to the pool, deletes them and
// atomically decrements spec.replicas by the number removed. The decrement is
// done on the main resource (server-side, from the apiserver's own identity),
// so it bypasses the /scale guard — that is what makes addressed removal work
// for Explicit pools.
type ScaleDownWithREST struct {
	client virtclient.Interface
}

var (
	_ rest.Storage      = &ScaleDownWithREST{}
	_ rest.NamedCreater = &ScaleDownWithREST{}
)

func NewScaleDownWithREST(c virtclient.Interface) *ScaleDownWithREST {
	return &ScaleDownWithREST{client: c}
}

func (r *ScaleDownWithREST) New() runtime.Object {
	return &subresources.VirtualMachinePoolScaleDownWith{}
}

func (r *ScaleDownWithREST) Destroy() {}

// Create implements rest.NamedCreater. The client POSTs a
// VirtualMachinePoolScaleDownWith to .../virtualmachinepools/<name>/scaledownwith,
// where name is the pool. Unlike the VM subresources, this handler does not proxy
// to another API server, so a plain create is enough — there is nothing to stream.
func (r *ScaleDownWithREST) Create(ctx context.Context, name string, obj runtime.Object, createValidation rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	body, ok := obj.(*subresources.VirtualMachinePoolScaleDownWith)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected VirtualMachinePoolScaleDownWith, got %T", obj))
	}
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}
	if len(body.Targets) == 0 {
		return nil, apierrors.NewBadRequest("scaleDownWith requires a non-empty targets list")
	}

	namespace := genericapirequest.NamespaceValue(ctx)
	if err := r.scaleDown(ctx, namespace, name, body.Targets); err != nil {
		return nil, err
	}
	return &metav1.Status{Status: metav1.StatusSuccess}, nil
}

func (r *ScaleDownWithREST) scaleDown(ctx context.Context, namespace, poolName string, targets []string) error {
	vms := r.client.VirtualizationV1alpha2().VirtualMachines(namespace)
	pools := r.client.VirtualizationV1alpha2().VirtualMachinePools(namespace)

	// Reads go straight to the API server (no cache): scaleDownWith is a rare,
	// user-initiated mutation, and validating targets before deleting them must
	// not observe stale membership.
	pool, err := pools.Get(ctx, poolName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(v1alpha2.Resource(v1alpha2.VirtualMachinePoolResource), poolName)
		}
		return apierrors.NewInternalError(err)
	}

	// Validate all targets up front: every one must be a member of this pool.
	// Fail the whole request if any is not, so we never partially delete.
	for _, target := range targets {
		vm, err := vms.Get(ctx, target, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return apierrors.NewBadRequest(fmt.Sprintf("target VirtualMachine %q not found in namespace %q", target, namespace))
			}
			return apierrors.NewInternalError(err)
		}
		if ref := metav1.GetControllerOf(vm); ref == nil || ref.UID != pool.GetUID() {
			return apierrors.NewBadRequest(fmt.Sprintf("target VirtualMachine %q does not belong to VirtualMachinePool %q", target, poolName))
		}
	}

	// Delete the targets. A target already gone still counts toward the decrement.
	for _, target := range targets {
		if err := vms.Delete(ctx, target, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return apierrors.NewInternalError(fmt.Errorf("delete target %q: %w", target, err))
		}
	}

	// Atomically shrink the pool by the number of removed replicas.
	removed := int32(len(targets))
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := pools.Get(ctx, poolName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		desired := int32(0)
		if current.Spec.Replicas != nil {
			desired = *current.Spec.Replicas
		}
		desired -= removed
		if desired < 0 {
			desired = 0
		}
		current.Spec.Replicas = &desired
		_, err = pools.Update(ctx, current, metav1.UpdateOptions{})
		return err
	})
}
