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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type WaitForPVCImportStepDiskService interface {
	EnsurePVCImport(ctx context.Context, target *corev1.PersistentVolumeClaim, source *service.PVCImportSource, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error)
}

// PVCImportSourceProvider builds the PVCImportSource used by WaitForPVCImportStep.
// It is invoked lazily so the source can be derived from objects (Pod, CVI, VI)
// inside the step instead of being constructed by the data source up front.
type PVCImportSourceProvider func(ctx context.Context, vd *v1alpha2.VirtualDisk) (*service.PVCImportSource, error)

// WaitForPVCImportStep drives the import of data from DVCR (or another
// PVCImportSource) into the target PersistentVolumeClaim and reflects its
// progress in the VirtualDisk status. It is a no-op until the PVC has been
// created and reached the Bound phase.
type WaitForPVCImportStep struct {
	pvc            *corev1.PersistentVolumeClaim
	sourceProvider PVCImportSourceProvider
	disk           WaitForPVCImportStepDiskService
	client         client.Client
	cb             *conditions.ConditionBuilder
}

func NewWaitForPVCImportStep(
	pvc *corev1.PersistentVolumeClaim,
	sourceProvider PVCImportSourceProvider,
	disk WaitForPVCImportStepDiskService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *WaitForPVCImportStep {
	return &WaitForPVCImportStep{
		pvc:            pvc,
		sourceProvider: sourceProvider,
		disk:           disk,
		client:         client,
		cb:             cb,
	}
}

func (s WaitForPVCImportStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc == nil || s.pvc.Status.Phase != corev1.ClaimBound {
		return nil, nil
	}

	nodePlacement, err := GetNodePlacement(ctx, s.client, vd)
	if err != nil {
		return nil, fmt.Errorf("failed to get importer tolerations: %w", err)
	}

	var source *service.PVCImportSource
	if s.sourceProvider != nil {
		source, err = s.sourceProvider(ctx, vd)
		if err != nil {
			return nil, fmt.Errorf("build pvc import source: %w", err)
		}
	}

	phase, err := s.disk.EnsurePVCImport(ctx, s.pvc, source, vd, nodePlacement)
	if err != nil {
		return nil, fmt.Errorf("ensure pvc import: %w", err)
	}

	switch phase {
	case corev1.PodSucceeded:
		return &reconcile.Result{RequeueAfter: time.Second}, nil
	case corev1.PodFailed:
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.Status(metav1.ConditionFalse).Reason(vdcondition.ProvisioningFailed).Message("VirtualDisk importer Pod failed.")
	default:
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.Status(metav1.ConditionFalse).Reason(vdcondition.Provisioning).Message("Import is in the process of provisioning to the PersistentVolumeClaim.")
	}
	return &reconcile.Result{}, nil
}

// StaticPVCImportSource returns a PVCImportSourceProvider that always returns
// the given source. It is useful when the source is fully known up front, e.g.
// for ObjectRef-based data sources.
func StaticPVCImportSource(source *service.PVCImportSource) PVCImportSourceProvider {
	return func(_ context.Context, _ *v1alpha2.VirtualDisk) (*service.PVCImportSource, error) {
		return source, nil
	}
}

// DVCRPodPVCImportSource returns a PVCImportSourceProvider that builds a
// registry-backed PVCImportSource using the DVCR image name resolved from the
// provided helper Pod (uploader or importer).
func DVCRPodPVCImportSource(pod *corev1.Pod, stat interface {
	GetDVCRImageName(pod *corev1.Pod) string
},
) PVCImportSourceProvider {
	return func(_ context.Context, vd *v1alpha2.VirtualDisk) (*service.PVCImportSource, error) {
		if pod == nil {
			return nil, nil
		}
		dvcrImageName := stat.GetDVCRImageName(pod)
		if dvcrImageName == "" {
			return nil, nil
		}
		return BuildDVCRPVCImportSource(vd, dvcrImageName), nil
	}
}
