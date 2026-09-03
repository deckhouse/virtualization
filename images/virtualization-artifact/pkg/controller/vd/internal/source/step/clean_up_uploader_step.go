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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// reached a final state (Ready, Lost or Exporting), or once the uploader
// delivered a terminal verdict (e.g. a checksum mismatch). It is a no-op while
// the disk is still being provisioned and when there is nothing left to clean up.
type CleanUpUploaderStep struct {
	pvc      *corev1.PersistentVolumeClaim
	pod      *corev1.Pod
	svc      *corev1.Service
	ing      *netv1.Ingress
	uploader CleanUpUploaderStepUploaderService
	cb       *conditions.ConditionBuilder
}

func NewCleanUpUploaderStep(
	pvc *corev1.PersistentVolumeClaim,
	pod *corev1.Pod,
	svc *corev1.Service,
	ing *netv1.Ingress,
	uploader CleanUpUploaderStepUploaderService,
	cb *conditions.ConditionBuilder,
) *CleanUpUploaderStep {
	return &CleanUpUploaderStep{
		pvc:      pvc,
		pod:      pod,
		svc:      svc,
		ing:      ing,
		uploader: uploader,
		cb:       cb,
	}
}

func (s CleanUpUploaderStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	condition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)

	// The uploader delivered a terminal verdict before the PVC came to be (e.g.
	// a checksum mismatch): re-uploading replays it, so keep the failure, clean
	// the uploader up and stop the pipeline - the create step downstream would
	// otherwise recreate the uploader and silently restart the failed import.
	if s.pvc == nil &&
		condition.Reason == vdcondition.ProvisioningFailedTerminally.String() {
		if s.pod != nil || s.svc != nil || s.ing != nil {
			supgen := vdsupplements.NewGenerator(vd)
			if _, err := s.uploader.CleanUp(ctx, supgen); err != nil {
				return nil, fmt.Errorf("clean up uploader supplements: %w", err)
			}
		}

		vd.Status.Phase = v1alpha2.DiskFailed
		vd.Status.ImageUploadURLs = nil
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailedTerminally).
			Message(condition.Message)

		return &reconcile.Result{}, nil
	}

	if s.pod == nil && s.svc == nil && s.ing == nil {
		return nil, nil
	}

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
