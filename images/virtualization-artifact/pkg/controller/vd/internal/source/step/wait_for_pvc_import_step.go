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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// pvcImportProgressRequeue is how often the step refreshes vd.Status.Progress
// from the cdi-importer pod metrics while the import is in flight.
const pvcImportProgressRequeue = 2 * time.Second

// WaitForPVCImportStepStatService is the subset of StatService used to extract
// the cdi-importer pod's progress and project it into vd.Status.Progress.
type WaitForPVCImportStepStatService interface {
	GetProgress(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...service.GetProgressOption) string
}

// PVCImportSourceProvider builds the PVCImportSource used by WaitForPVCImportStep.
// It is invoked lazily so the source can be derived from objects (Pod, CVI, VI)
// inside the step instead of being constructed by the data source up front.
type PVCImportSourceProvider func(ctx context.Context, vd *v1alpha2.VirtualDisk) (*service.PVCImportSource, error)

// WaitForPVCImportStep drives the import of data from DVCR (or another
// PVCImportSource) into the target PersistentVolumeClaim and reflects its
// progress in the VirtualDisk status. It is a no-op until the PVC has been
// created and reached the Bound phase.
//
// While the import is running the step requeues every pvcImportProgressRequeue
// and republishes vd.Status.Progress from the cdi-importer pod's
// kubevirt_cdi_import_progress_total metric (0..100). When progressScale is
// set, the value is projected into the [progressScale.Low, progressScale.High]
// slice of the disk-wide progress (e.g. 50..100 for HTTP / Registry / Upload
// data sources where the first 50% is already filled by the DVCR phase).
type WaitForPVCImportStep struct {
	pvc            *corev1.PersistentVolumeClaim
	sourceProvider PVCImportSourceProvider
	pvcSvc         PVCService
	stat           WaitForPVCImportStepStatService
	progressScale  *service.ScaleOption
	client         client.Client
	cb             *conditions.ConditionBuilder
}

func NewWaitForPVCImportStep(
	pvc *corev1.PersistentVolumeClaim,
	sourceProvider PVCImportSourceProvider,
	pvcSvc PVCService,
	stat WaitForPVCImportStepStatService,
	progressScale *service.ScaleOption,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *WaitForPVCImportStep {
	return &WaitForPVCImportStep{
		pvc:            pvc,
		sourceProvider: sourceProvider,
		pvcSvc:         pvcSvc,
		stat:           stat,
		progressScale:  progressScale,
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

	sup := vdsupplements.NewGenerator(vd)
	phase, err := s.pvcSvc.Import(ctx, s.pvc, source, vd, sup, nodePlacement)
	if err != nil {
		return nil, fmt.Errorf("pvc import: %w", err)
	}

	switch phase {
	case corev1.PodSucceeded:
		return &reconcile.Result{RequeueAfter: time.Second}, nil
	case corev1.PodFailed:
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.Status(metav1.ConditionFalse).Reason(vdcondition.ProvisioningFailed).Message("VirtualDisk importer Pod failed.")
		return &reconcile.Result{}, nil
	default:
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.Status(metav1.ConditionFalse).Reason(vdcondition.Provisioning).Message("Import is in the process of provisioning to the PersistentVolumeClaim.")

		if err := s.refreshProgressFromPod(ctx, vd); err != nil {
			return nil, err
		}

		return &reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}
}

// refreshProgressFromPod queries the cdi-importer pod (named after the target
// PVC) for its progress metric and updates vd.Status.Progress. Silently keeps
// the previous value when stat/pod is missing or metrics are not yet readable.
func (s WaitForPVCImportStep) refreshProgressFromPod(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if s.stat == nil {
		return nil
	}

	pod, err := object.FetchObject(ctx, types.NamespacedName{Name: s.pvc.Name, Namespace: s.pvc.Namespace}, s.client, &corev1.Pod{})
	if err != nil {
		return fmt.Errorf("fetch cdi-importer pod: %w", err)
	}
	if pod == nil {
		return nil
	}

	var opts []service.GetProgressOption
	if s.progressScale != nil {
		opts = append(opts, s.progressScale)
	}
	vd.Status.Progress = s.stat.GetProgress(vd.GetUID(), pod, vd.Status.Progress, opts...)
	return nil
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

// ClusterVirtualImagePVCImportSource returns a PVCImportSourceProvider that
// fetches the referenced ClusterVirtualImage on demand and builds a
// registry-backed PVCImportSource. The fetch happens only when the step
// actually needs the source (i.e. while the import is in flight), so the data
// source handler does not have to fetch the CVI on every reconcile.
func ClusterVirtualImagePVCImportSource(c client.Client) PVCImportSourceProvider {
	return func(ctx context.Context, vd *v1alpha2.VirtualDisk) (*service.PVCImportSource, error) {
		if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
			return nil, nil
		}
		key := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name}
		cviRef, err := object.FetchObject(ctx, key, c, &v1alpha2.ClusterVirtualImage{})
		if err != nil {
			return nil, fmt.Errorf("fetch cvi %q: %w", key, err)
		}
		if cviRef == nil {
			return nil, nil
		}
		return BuildClusterVirtualImagePVCImportSource(vd, cviRef), nil
	}
}

// VirtualImagePVCImportSource returns a PVCImportSourceProvider that fetches
// the referenced VirtualImage on demand and builds the appropriate
// PVCImportSource (registry-backed for DVCR storage, PVC-backed for in-cluster
// PVC storage). The fetch happens only when the step actually needs the
// source.
func VirtualImagePVCImportSource(c client.Client) PVCImportSourceProvider {
	return func(ctx context.Context, vd *v1alpha2.VirtualDisk) (*service.PVCImportSource, error) {
		if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
			return nil, nil
		}
		key := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}
		viRef, err := object.FetchObject(ctx, key, c, &v1alpha2.VirtualImage{})
		if err != nil {
			return nil, fmt.Errorf("fetch vi %q: %w", key, err)
		}
		if viRef == nil {
			return nil, nil
		}
		return BuildVirtualImagePVCImportSource(vd, viRef)
	}
}
