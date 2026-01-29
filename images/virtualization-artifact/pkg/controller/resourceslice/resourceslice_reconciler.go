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

package resourceslice

import (
	"context"
	"fmt"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	draDriverName = "virtualization-dra"
)

type Handler interface {
	Handle(ctx context.Context, slice *resourcev1beta1.ResourceSlice) error
	Name() string
}

type Reconciler struct {
	client   client.Client
	handlers []Handler
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	slice := &resourcev1beta1.ResourceSlice{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: req.Name}, slice); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("get ResourceSlice %s: %w", req.Name, err)
	}

	if slice.Spec.Driver != draDriverName {
		return reconcile.Result{}, nil
	}

	for _, h := range r.handlers {
		if err := h.Handle(ctx, slice); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
