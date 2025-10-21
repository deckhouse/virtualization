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
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CreateDataVolumeFromClusterVirtualImageStep struct {
	pvc    *corev1.PersistentVolumeClaim
	dv     *cdiv1.DataVolume
	disk   CreateDataVolumeStepDiskService
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewCreateDataVolumeFromClusterVirtualImageStep(
	pvc *corev1.PersistentVolumeClaim,
	dv *cdiv1.DataVolume,
	disk CreateDataVolumeStepDiskService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreateDataVolumeFromClusterVirtualImageStep {
	return &CreateDataVolumeFromClusterVirtualImageStep{
		pvc:    pvc,
		dv:     dv,
		disk:   disk,
		client: client,
		cb:     cb,
	}
}

func (s CreateDataVolumeFromClusterVirtualImageStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil || s.dv != nil {
		return nil, nil
	}

	cviRefKey := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name}
	cviRef, err := object.FetchObject(ctx, cviRefKey, s.client, &v1alpha2.ClusterVirtualImage{})
	if err != nil {
		return nil, fmt.Errorf("fetch cvi %q: %w", cviRefKey, err)
	}

	if cviRef == nil {
		return nil, fmt.Errorf("cvi object ref %q is nil", cviRefKey)
	}

	vd.Status.SourceUID = ptr.To(cviRef.UID)

	if imageformat.IsISO(cviRef.Status.Format) {
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(ErrISOSourceNotSupported.Error()) + ".")
		return &reconcile.Result{}, nil
	}

	source := s.getSource(vd, cviRef)

	size, err := s.getPVCSize(vd, cviRef)
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

func (s CreateDataVolumeFromClusterVirtualImageStep) getPVCSize(vd *v1alpha2.VirtualDisk, cviRef *v1alpha2.ClusterVirtualImage) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(cviRef.Status.Size.UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", cviRef.Status.Size.UnpackedBytes, err)
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}

func (s CreateDataVolumeFromClusterVirtualImageStep) getSource(vd *v1alpha2.VirtualDisk, cviRef *v1alpha2.ClusterVirtualImage) *cdiv1.DataVolumeSource {
	supgen := vdsupplements.NewGenerator(vd)

	url := common.DockerRegistrySchemePrefix + cviRef.Status.Target.RegistryURL
	secretName := supgen.DVCRAuthSecretForDV().Name
	certConfigMapName := supgen.DVCRCABundleConfigMapForDV().Name

	return &cdiv1.DataVolumeSource{
		Registry: &cdiv1.DataVolumeSourceRegistry{
			URL:           &url,
			SecretRef:     &secretName,
			CertConfigMap: &certConfigMapName,
		},
	}
}
