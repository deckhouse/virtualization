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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type DisplayConditionHandler struct{}

func NewDisplayConditionHandler() *DisplayConditionHandler {
	return &DisplayConditionHandler{}
}

func (h DisplayConditionHandler) Handle(_ context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if vd.Status.Phase == virtv2.DiskReady {
		conditions.RemoveCondition(vdcondition.DatasourceReadyType, &vd.Status.Conditions)
	}

	return reconcile.Result{}, nil
}
