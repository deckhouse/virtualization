/*
Copyright 2025 Flant JSC

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
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type InitHandler struct{}

func NewInitHandler() *InitHandler {
	return &InitHandler{}
}

func (h *InitHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	// INIT PersistentVolumeClaim Name.
	// Required for correct work virtual disk supplements.
	// We should have different names for support migration volumes.
	// If the PVC name is empty, we should generate it and update the status immediately.
	if vd.Status.Target.PersistentVolumeClaim == "" {
		name := fmt.Sprintf("d8v-vd-%s-%s", vd.UID, pwgen.LowerAlpha(5))
		vdsupplements.SetPVCName(vd, name)
		return reconcile.Result{RequeueAfter: 100 * time.Millisecond}, reconciler.ErrStopHandlerChain
	}
	return reconcile.Result{}, nil
}
