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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// pvcImportProgressRequeue is how often the step refreshes vd.Status.Progress
// from the pvc-importer pod metrics while the import is in flight.
const pvcImportProgressRequeue = 2 * time.Second

// WaitForPVCImportStepStatService is the subset of StatService used to extract
// the pvc-importer pod's progress and project it into vd.Status.Progress.
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
// and republishes vd.Status.Progress from the pvc-importer pod's
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
	if vd.Status.Progress == "" {
		vd.Status.Progress = "0%"
	}

	if s.pvc == nil {
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Waiting for the underlying PersistentVolumeClaim to be created.")
		return &reconcile.Result{}, nil
	}

	// Note: the target PVC is intentionally NOT required to be Bound here. The
	// importer fills a separate prime PVC and its volume is rebound to the target on
	// completion (see PVCImporterService.Wait/Rebind), so the target only becomes
	// Bound at the very end of the import. Gating the import on the target being
	// Bound would deadlock (the target waits for the rebind, the rebind waits for the
	// import). The import and the rebind are driven by Import/WaitForImport below.

	waiting, err := s.awaitingFirstConsumer(ctx, vd)
	if err != nil {
		return nil, err
	}
	if waiting {
		vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.WaitingForFirstConsumer).
			Message("Awaiting the scheduling of the VirtualMachine with the attached VirtualDisk.")
		return &reconcile.Result{}, nil
	}

	phase, err := s.pvcImportPodPhase(ctx, vd)
	if err != nil {
		return nil, err
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

// awaitingFirstConsumer reports whether the target PVC exists on a
// WaitForFirstConsumer storage class but no consumer has scheduled onto it yet:
// it is unbound, unpopulated, carries no selected-node annotation and the
// consuming VirtualMachine has no node. The populator defers the import until
// the scheduler stamps selected-node (set for the VM's launcher pod, KubeVirt's
// temporary first-consumer pod, or a hotplug attachment pod), so until then the
// disk must keep reporting WaitForFirstConsumer: the VirtualMachine controller
// only starts the VM that produces that consumer while the disk is in this
// phase.
func (s WaitForPVCImportStep) awaitingFirstConsumer(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	if s.pvc.Status.Phase == corev1.ClaimBound ||
		s.pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" ||
		s.pvc.Annotations[service.SelectedNodeAnnotation] != "" {
		return false, nil
	}

	wffc, err := isStorageClassWFFC(ctx, s.client, ptr.Deref(s.pvc.Spec.StorageClassName, ""))
	if err != nil {
		return false, err
	}
	if !wffc {
		return false, nil
	}

	nodePlacement, err := commonvd.GetNodePlacement(ctx, s.client, vd)
	if err != nil {
		return false, err
	}
	return nodePlacement == nil || nodePlacement.Node == "", nil
}

func (s WaitForPVCImportStep) pvcImportPodPhase(ctx context.Context, vd *v1alpha2.VirtualDisk) (corev1.PodPhase, error) {
	if s.pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		return corev1.PodSucceeded, nil
	}
	pod, err := s.fetchPVCImportPod(ctx, vd)
	if err != nil {
		return "", fmt.Errorf("fetch pvc-importer pod: %w", err)
	}
	if pod == nil || pod.Status.Phase == "" {
		return corev1.PodPending, nil
	}
	return pod.Status.Phase, nil
}

// refreshProgressFromPod queries the pvc-importer pod (named after the target
// PVC) for its progress metric and updates vd.Status.Progress. Silently keeps
// the previous value when stat/pod is missing or metrics are not yet readable.
func (s WaitForPVCImportStep) refreshProgressFromPod(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if s.stat == nil {
		return nil
	}

	pod, err := s.fetchPVCImportPod(ctx, vd)
	if err != nil {
		return fmt.Errorf("fetch pvc-importer pod: %w", err)
	}
	if pod == nil {
		return nil
	}

	var opts []service.GetProgressOption
	if s.progressScale != nil {
		opts = append(opts, s.progressScale)
	}
	vd.Status.Progress = s.stat.GetProgress(vd.GetUID(), pod, vd.Status.Progress, opts...)
	vd.Status.Progress = service.CapProgressBelow(vd.Status.Progress, 100)
	return nil
}

func (s WaitForPVCImportStep) fetchPVCImportPod(ctx context.Context, vd *v1alpha2.VirtualDisk) (*corev1.Pod, error) {
	sup := vdsupplements.NewGenerator(vd)
	if s.pvc.Annotations[annotations.AnnPVCPopulationStrategy] == service.PopulationStrategyHostAssigned {
		return object.FetchObject(ctx, types.NamespacedName{Name: sup.PVCTargetImporterPod().Name, Namespace: s.pvc.Namespace}, s.client, &corev1.Pod{})
	}
	return object.FetchObject(ctx, types.NamespacedName{Name: sup.PVCImporterPod().Name, Namespace: s.pvc.Namespace}, s.client, &corev1.Pod{})
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
