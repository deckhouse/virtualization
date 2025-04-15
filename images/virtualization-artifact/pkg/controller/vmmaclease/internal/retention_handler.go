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

package internal

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmaclease/internal/state"
)

const RetentionHandlerName = "RetentionHandler"

type RetentionHandler struct {
}

func NewRetentionHandler() *RetentionHandler {
	return &RetentionHandler{}
}

func (h *RetentionHandler) Handle(ctx context.Context, state state.VMMACLeaseState) (reconcile.Result, error) {
	mac, err := state.VirtualMachineMACAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if mac == nil {
		state.SetDeletion(true)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (h *RetentionHandler) Name() string {
	return RetentionHandlerName
}
