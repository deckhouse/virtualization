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

package step

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const terminatingStep = "terminating"

type TerminatingStep struct {
	pvc *corev1.PersistentVolumeClaim
}

func NewTerminatingStep(pvc *corev1.PersistentVolumeClaim) *TerminatingStep {
	return &TerminatingStep{
		pvc: pvc,
	}
}

func (s TerminatingStep) Take(ctx context.Context, _ *virtv2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc == nil {
		return nil, nil
	}

	if object.IsTerminating(s.pvc) {
		log := logger.FromContext(ctx).With(logger.SlogStep(terminatingStep))

		log.Debug("Wait until PVC will be terminated")

		return &reconcile.Result{RequeueAfter: 2 * time.Second}, nil
	}

	return nil, nil
}
