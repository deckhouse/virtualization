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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ImagePresenceHandler struct {
	client       client.Client
	dvcrSettings *dvcr.Settings
}

func NewImagePresenceHandler(client client.Client, dvcrSettings *dvcr.Settings) *ImagePresenceHandler {
	return &ImagePresenceHandler{
		client:       client,
		dvcrSettings: dvcrSettings,
	}
}

func (h *ImagePresenceHandler) Name() string {
	return "ImagePresenceHandler"
}

func (h *ImagePresenceHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	if vi.Status.Phase != v1alpha2.ImageReady {
		return reconcile.Result{}, nil
	}

	if vi.Spec.Storage != v1alpha2.StorageContainerRegistry {
		return reconcile.Result{}, nil
	}

	registryURL := vi.Status.Target.RegistryURL
	if registryURL == "" {
		return reconcile.Result{}, nil
	}

	exists, err := dvcr.NewImageChecker(h.client, h.dvcrSettings).CheckImageExists(ctx, registryURL)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to check image existence in DVCR: %w", err)
	}

	if !exists {
		vi.Status.Phase = v1alpha2.ImageLost

		cb := conditions.NewConditionBuilder(vicondition.ReadyType).
			Generation(vi.Generation).
			Status(metav1.ConditionFalse).
			Reason(vicondition.ImageLost).
			Message(fmt.Sprintf("Image %q not found in DVCR.", registryURL))

		conditions.SetCondition(cb, &vi.Status.Conditions)
	}

	return reconcile.Result{}, nil
}
