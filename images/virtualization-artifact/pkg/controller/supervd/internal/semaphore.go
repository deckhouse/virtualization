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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	vdcontroller "github.com/deckhouse/virtualization-controller/pkg/controller/vd"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SemaphoreHandler struct {
	mgr                  manager.Manager
	logFactory           logger.Factory
	importerImage        string
	uploaderImage        string
	requirements         corev1.ResourceRequirements
	dvcr                 *dvcr.Settings
	storageClassSettings config.VirtualDiskStorageClassSettings

	controllers map[string]chan reconcile.Request
}

func NewSemaphoreHandler(
	mgr manager.Manager,
	logFactory logger.Factory,
	importerImage string,
	uploaderImage string,
	requirements corev1.ResourceRequirements,
	dvcr *dvcr.Settings,
	storageClassSettings config.VirtualDiskStorageClassSettings,
) *SemaphoreHandler {
	return &SemaphoreHandler{
		mgr:                  mgr,
		logFactory:           logFactory,
		importerImage:        importerImage,
		uploaderImage:        uploaderImage,
		requirements:         requirements,
		dvcr:                 dvcr,
		storageClassSettings: storageClassSettings,
		controllers:          make(map[string]chan reconcile.Request),
	}
}

func (h *SemaphoreHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	logger.FromContext(ctx).Warn("[test][SUPER] STARTED")

	queue, ok := h.controllers[vd.Namespace]
	if !ok {
		logger.FromContext(ctx).Warn("[test][SUPER] START NEW NAMESPACED CONTROLLER")

		queue = make(chan reconcile.Request)

		_, err := vdcontroller.NewController(
			ctx,
			h.mgr,
			h.logFactory(vdcontroller.ControllerName+"-"+vd.Namespace),
			h.importerImage,
			h.uploaderImage,
			h.requirements,
			h.dvcr,
			h.storageClassSettings,
			vd.Namespace,
			queue,
		)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("setup vm controller for %q: %w", vd.Namespace, err)
		}

		h.controllers[vd.Namespace] = queue
	}

	logger.FromContext(ctx).Warn("[test][SUPER] PUSH REQUEST TO QUEUE")

	queue <- reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      vd.Name,
		Namespace: vd.Namespace,
	}}

	logger.FromContext(ctx).Warn("[test][SUPER] FINISHED")

	return reconcile.Result{}, nil
}
