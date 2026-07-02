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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

// imageLostRecheckInterval is how often DVCR is polled while an image is lost,
// so recovery is noticed shortly after the data (for example, a DVCR PVC) returns.
const imageLostRecheckInterval = 30 * time.Second

type ImagePresenceHandler struct {
	imageChecker dvcr.ImageChecker
	recorder     eventrecord.EventRecorderLogger
}

func NewImagePresenceHandler(recorder eventrecord.EventRecorderLogger, imageChecker dvcr.ImageChecker) *ImagePresenceHandler {
	return &ImagePresenceHandler{
		imageChecker: imageChecker,
		recorder:     recorder,
	}
}

func (h *ImagePresenceHandler) Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	phase := cvi.Status.Phase
	if phase != v1alpha2.ImageReady && phase != v1alpha2.ImageLost {
		return reconcile.Result{}, nil
	}

	registryURL := cvi.Status.Target.RegistryURL
	if registryURL == "" {
		return reconcile.Result{}, nil
	}

	exists, err := h.imageChecker.CheckImageExists(ctx, registryURL)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to check image existence in DVCR: %w", err)
	}

	if !exists {
		if phase == v1alpha2.ImageReady {
			cvi.Status.Phase = v1alpha2.ImageLost

			cb := conditions.NewConditionBuilder(cvicondition.ReadyType).
				Generation(cvi.Generation).
				Status(metav1.ConditionFalse).
				Reason(cvicondition.ImageLost).
				Message("The image data is no longer available and needs to be recreated.")

			conditions.SetCondition(cb, &cvi.Status.Conditions)
		}

		// Keep polling: the data may return (for example, when the DVCR PVC is remounted).
		return reconcile.Result{RequeueAfter: imageLostRecheckInterval}, nil
	}

	if phase == v1alpha2.ImageLost {
		h.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonCVIImageLostRecovered,
			"The image reappeared in DVCR and was restored to Ready.",
		)

		cvi.Status.Phase = v1alpha2.ImageReady

		cb := conditions.NewConditionBuilder(cvicondition.ReadyType).
			Generation(cvi.Generation).
			Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		conditions.SetCondition(cb, &cvi.Status.Conditions)
	}

	return reconcile.Result{}, nil
}
