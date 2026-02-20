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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CreateDataVolumeFromVirtualImageStep struct {
	pvc    *corev1.PersistentVolumeClaim
	dv     *cdiv1.DataVolume
	disk   CreateDataVolumeStepDiskService
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewCreateDataVolumeFromVirtualImageStep(
	pvc *corev1.PersistentVolumeClaim,
	dv *cdiv1.DataVolume,
	disk CreateDataVolumeStepDiskService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreateDataVolumeFromVirtualImageStep {
	return &CreateDataVolumeFromVirtualImageStep{
		pvc:    pvc,
		dv:     dv,
		disk:   disk,
		client: client,
		cb:     cb,
	}
}

func (s CreateDataVolumeFromVirtualImageStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil || s.dv != nil {
		return nil, nil
	}

	_, exists := vd.Annotations[annotations.AnnUseVolumeSnapshot]
	if exists {
		return nil, nil
	}

	viRefKey := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}
	viRef, err := object.FetchObject(ctx, viRefKey, s.client, &v1alpha2.VirtualImage{})
	if err != nil {
		return nil, fmt.Errorf("fetch vi %q: %w", viRefKey, err)
	}

	if viRef == nil {
		return nil, fmt.Errorf("vi object ref %q is nil", viRefKey)
	}

	vd.Status.SourceUID = ptr.To(viRef.UID)

	if imageformat.IsISO(viRef.Status.Format) {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(ErrISOSourceNotSupported.Error()) + ".")
		return &reconcile.Result{}, nil
	}

	source, err := s.getSource(vd, viRef)
	if err != nil {
		return nil, fmt.Errorf("get source %q: %w", viRefKey, err)
	}

	size, err := s.getPVCSize(vd, viRef)
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

	return NewCreateDataVolumeStep(s.dv, s.disk, s.client, source, size, s.cb).Take(ctx, vd)
}

func (s CreateDataVolumeFromVirtualImageStep) getPVCSize(vd *v1alpha2.VirtualDisk, viRef *v1alpha2.VirtualImage) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(viRef.Status.Size.UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", viRef.Status.Size.UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}

func (s CreateDataVolumeFromVirtualImageStep) getSource(vd *v1alpha2.VirtualDisk, viRef *v1alpha2.VirtualImage) (*cdiv1.DataVolumeSource, error) {
	switch viRef.Spec.Storage {
	case v1alpha2.StoragePersistentVolumeClaim, v1alpha2.StorageKubernetes:
		return &cdiv1.DataVolumeSource{
			PVC: &cdiv1.DataVolumeSourcePVC{
				Name:      viRef.Status.Target.PersistentVolumeClaim,
				Namespace: viRef.Namespace,
			},
		}, nil
	case v1alpha2.StorageContainerRegistry, "":
		supgen := vdsupplements.NewGenerator(vd)

		url := common.DockerRegistrySchemePrefix + viRef.Status.Target.RegistryURL
		secretName := supgen.DVCRAuthSecretForDV().Name
		certConfigMapName := supgen.DVCRCABundleConfigMapForDV().Name

		return &cdiv1.DataVolumeSource{
			Registry: &cdiv1.DataVolumeSourceRegistry{
				URL:           &url,
				SecretRef:     &secretName,
				CertConfigMap: &certConfigMapName,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected virtual image storage %s, please report a bug", viRef.Spec.Storage)
	}
}
