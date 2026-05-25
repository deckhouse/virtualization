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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type StartImportFromDVCRStepStatService interface {
	GetSize(pod *corev1.Pod) v1alpha2.ImageStatusSize
	GetFormat(pod *corev1.Pod) string
	GetDVCRImageName(pod *corev1.Pod) string
	GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *v1alpha2.StatusSpeed
}

// StartImportFromDVCRStep starts the PVC import from DVCR once the helper Pod
// (uploader or importer) has finished populating DVCR. It is a no-op while the
// PVC already exists or the Pod has not yet succeeded.
type StartImportFromDVCRStep struct {
	pvc    *corev1.PersistentVolumeClaim
	pod    *corev1.Pod
	stat   StartImportFromDVCRStepStatService
	disk   PVCImportStepDiskService
	pvcSvc PVCService
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewStartImportFromDVCRStep(
	pvc *corev1.PersistentVolumeClaim,
	pod *corev1.Pod,
	stat StartImportFromDVCRStepStatService,
	disk PVCImportStepDiskService,
	pvcSvc PVCService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *StartImportFromDVCRStep {
	return &StartImportFromDVCRStep{
		pvc:    pvc,
		pod:    pod,
		stat:   stat,
		disk:   disk,
		pvcSvc: pvcSvc,
		client: client,
		cb:     cb,
	}
}

func (s StartImportFromDVCRStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil {
		return nil, nil
	}

	if !podutil.IsPodComplete(s.pod) {
		return nil, nil
	}

	vd.Status.Progress = "50%"
	vd.Status.DownloadSpeed = s.stat.GetDownloadSpeed(vd.GetUID(), s.pod)

	if imageformat.IsISO(s.stat.GetFormat(s.pod)) {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(ErrISOSourceNotSupported.Error()) + ".")
		return &reconcile.Result{}, nil
	}

	size, err := s.getPVCSize(vd)
	if err != nil {
		if errors.Is(err, service.ErrInsufficientPVCSize) {
			vd.Status.Phase = v1alpha2.DiskFailed
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningFailed).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			return &reconcile.Result{}, nil
		}

		return nil, err
	}

	source := BuildDVCRPVCImportSource(vd, s.stat.GetDVCRImageName(s.pod))

	return NewPVCImportStep(s.disk, s.pvcSvc, s.client, source, size, s.cb).Take(ctx, vd)
}

func (s StartImportFromDVCRStep) getPVCSize(vd *v1alpha2.VirtualDisk) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(s.stat.GetSize(s.pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", s.stat.GetSize(s.pod).UnpackedBytes, err)
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}

// BuildDVCRPVCImportSource constructs a PVCImportSource for a DVCR registry
// image. The image name is typically resolved from a helper Pod that uploaded
// or downloaded the data into DVCR.
func BuildDVCRPVCImportSource(vd *v1alpha2.VirtualDisk, dvcrImageName string) *service.PVCImportSource {
	supgen := vdsupplements.NewGenerator(vd)

	url := common.DockerRegistrySchemePrefix + dvcrImageName
	secretName := supgen.DVCRAuthSecretForDV().Name
	certConfigMapName := supgen.DVCRCABundleConfigMapForDV().Name

	return service.NewPVCRegistryImportSource(url, secretName, certConfigMapName)
}
