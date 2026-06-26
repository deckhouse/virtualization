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

package step

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CleanUpUploaderStepUploaderService interface {
	CleanUp(ctx context.Context, sup supplements.Generator) (bool, error)
}

// CleanUpUploaderStep deletes uploader Pod/Service/Ingress once the disk has
// reached a final state (Ready, Lost or Exporting). It is a no-op while the
// disk is still being provisioned and when there is nothing left to clean up.
type CleanUpUploaderStep struct {
	pod      *corev1.Pod
	svc      *corev1.Service
	ing      *netv1.Ingress
	uploader CleanUpUploaderStepUploaderService
}

func NewCleanUpUploaderStep(
	pod *corev1.Pod,
	svc *corev1.Service,
	ing *netv1.Ingress,
	uploader CleanUpUploaderStepUploaderService,
) *CleanUpUploaderStep {
	return &CleanUpUploaderStep{
		pod:      pod,
		svc:      svc,
		ing:      ing,
		uploader: uploader,
	}
}

func (s CleanUpUploaderStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pod == nil && s.svc == nil && s.ing == nil {
		return nil, nil
	}

	condition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !isDiskProvisioningFinished(condition.Reason) {
		return nil, nil
	}

	supgen := vdsupplements.NewGenerator(vd)
	if _, err := s.uploader.CleanUp(ctx, supgen); err != nil {
		return nil, fmt.Errorf("clean up uploader supplements: %w", err)
	}

	return nil, nil
}

// isDiskProvisioningFinished reports whether the disk has reached a terminal
// provisioning state: Ready, Lost, or Exporting.
func isDiskProvisioningFinished(reason string) bool {
	return reason == vdcondition.Ready.String() ||
		reason == vdcondition.Lost.String() ||
		reason == vdcondition.Exporting.String()
}
