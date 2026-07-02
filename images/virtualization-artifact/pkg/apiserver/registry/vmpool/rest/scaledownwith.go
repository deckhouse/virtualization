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
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	client client.Client
}

var (
	_ rest.Storage   = &ScaleDownWithREST{}
	_ rest.Connecter = &ScaleDownWithREST{}
)

func NewScaleDownWithREST(c client.Client) *ScaleDownWithREST {
	return &ScaleDownWithREST{client: c}
}

func (r *ScaleDownWithREST) New() runtime.Object {
	return &subresources.VirtualMachinePoolScaleDownWith{}
}

func (r *ScaleDownWithREST) Destroy() {}

// NewConnectOptions implements rest.Connecter.
func (r *ScaleDownWithREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachinePoolScaleDownWith{}, false, ""
}

// ConnectMethods implements rest.Connecter.
func (r *ScaleDownWithREST) ConnectMethods() []string {
	return []string{http.MethodPost}
}

func (r *ScaleDownWithREST) Connect(ctx context.Context, name string, _ runtime.Object, responder rest.Responder) (http.Handler, error) {
	namespace := genericapirequest.NamespaceValue(ctx)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body subresources.VirtualMachinePoolScaleDownWith
		if req.Body != nil {
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				responder.Error(apierrors.NewBadRequest(fmt.Sprintf("decode scaleDownWith body: %v", err)))
				return
			}
		}
		if len(body.Targets) == 0 {
			responder.Error(apierrors.NewBadRequest("scaleDownWith requires a non-empty targets list"))
			return
		}

		if err := r.scaleDown(req.Context(), namespace, name, body.Targets); err != nil {
			responder.Error(err)
			return
		}
		responder.Object(http.StatusOK, &metav1.Status{Status: metav1.StatusSuccess})
	}), nil
}

func (r *ScaleDownWithREST) scaleDown(ctx context.Context, namespace, poolName string, targets []string) error {
	pool := &v1alpha2.VirtualMachinePool{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: poolName}, pool); err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(v1alpha2.Resource(v1alpha2.VirtualMachinePoolResource), poolName)
		}
		return apierrors.NewInternalError(err)
	}

	// Validate all targets up front: every one must be a member of this pool.
	// Fail the whole request if any is not, so we never partially delete.
	for _, target := range targets {
		vm := &v1alpha2.VirtualMachine{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: target}, vm); err != nil {
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
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: target}}
		if err := r.client.Delete(ctx, vm); err != nil && !apierrors.IsNotFound(err) {
			return apierrors.NewInternalError(fmt.Errorf("delete target %q: %w", target, err))
		}
	}

	// Atomically shrink the pool by the number of removed replicas.
	removed := int32(len(targets))
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha2.VirtualMachinePool{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: poolName}, current); err != nil {
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
		return r.client.Update(ctx, current)
	})
}
