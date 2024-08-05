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

package two_phase_reconciler

import (
	"context"
	"log/slog"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerStateFactory[T ReconcilerState] func(name types.NamespacedName, logger *slog.Logger, client client.Client, cache cache.Cache) T

type ReconcilerState interface {
	ReconcilerStateApplier

	SetReconcilerResult(result *reconcile.Result)
	GetReconcilerResult() *reconcile.Result

	Reload(ctx context.Context, req reconcile.Request, logger *slog.Logger, client client.Client) error
	ShouldReconcile(log *slog.Logger) bool
}

type ReconcilerStateApplier interface {
	ApplySync(ctx context.Context, log *slog.Logger) error
	ApplyUpdateStatus(ctx context.Context, log *slog.Logger) error
}
