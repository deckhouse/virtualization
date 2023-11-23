package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	vmiutil "github.com/deckhouse/virtualization-controller/pkg/common/vmi"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type VMIReconcilerState struct {
	*vmattachee.AttacheeState[*virtv2.VirtualMachineImage, virtv2.VirtualMachineImageStatus]

	Client      client.Client
	Supplements *supplements.Generator
	Result      *reconcile.Result

	VMI            *helper.Resource[*virtv2.VirtualMachineImage, virtv2.VirtualMachineImageStatus]
	DV             *cdiv1.DataVolume
	PVC            *corev1.PersistentVolumeClaim
	PV             *corev1.PersistentVolume
	Pod            *corev1.Pod
	DVCRDataSource *DataSourcesFromDVCR
	Service        *corev1.Service
	AttachedVMs    []*virtv2.VirtualMachine
}

func NewVMIReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMIReconcilerState {
	state := &VMIReconcilerState{
		Client: client,
		VMI: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineImage { return &virtv2.VirtualMachineImage{} },
			func(obj *virtv2.VirtualMachineImage) virtv2.VirtualMachineImageStatus { return obj.Status },
		),
	}

	state.AttacheeState = vmattachee.NewAttacheeState(
		state,
		"vmi",
		virtv2.FinalizerVMIProtection,
		state.VMI,
	)

	return state
}

func (state *VMIReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VMI.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMI %q meta: %w", state.VMI.Name(), err)
	}
	return nil
}

func (state *VMIReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.VMI.UpdateStatus(ctx)
}

func (state *VMIReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMIReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMIReconcilerState) ShouldApplyUpdateStatus() bool {
	return state.VMI.IsStatusChanged()
}

func (state *VMIReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	if err := state.VMI.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.VMI.IsEmpty() {
		log.Info("Reconcile observe an absent VMI: it may be deleted", "vmi.name", req.Name, "vmi.namespace", req.Namespace)
		return nil
	}

	state.Supplements = &supplements.Generator{
		Prefix:    vmiShortName,
		Name:      state.VMI.Current().Name,
		Namespace: state.VMI.Current().Namespace,
		UID:       state.VMI.Current().UID,
	}

	switch state.VMI.Current().Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		pod, err := uploader.FindPod(ctx, client, state.VMI.Current())
		if err != nil && !errors.Is(err, uploader.ErrPodNameNotFound) {
			return err
		}
		state.Pod = pod

		service, err := uploader.FindService(ctx, client, state.VMI.Current())
		if err != nil && !errors.Is(err, uploader.ErrServiceNameNotFound) {
			return err
		}
		state.Service = service
	default:
		pod, err := importer.FindPod(ctx, client, state.VMI.Current())
		if err != nil && !errors.Is(err, importer.ErrPodNameNotFound) {
			return err
		}
		state.Pod = pod
		dsDvcr, err := NewDVCRDataSource(ctx, state.VMI.Current().Spec.DataSource, state.VMI.Current(), client)
		if err != nil {
			return err
		}
		state.DVCRDataSource = dsDvcr
	}

	// TODO use status to save underlying DataVolume name.
	if dvName, hasKey := state.VMI.Current().Annotations[cc.AnnVMIDataVolume]; hasKey {
		var err error
		name := types.NamespacedName{Name: dvName, Namespace: state.VMI.Current().Namespace}

		state.DV, err = helper.FetchObject(ctx, name, client, &cdiv1.DataVolume{})
		if err != nil {
			return fmt.Errorf("unable to get DV %q: %w", name, err)
		}
		if state.DV != nil {
			switch MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase) {
			case virtv2.DiskProvisioning, virtv2.DiskReady:
				state.PVC, err = helper.FetchObject(ctx, name, client, &corev1.PersistentVolumeClaim{})
				if err != nil {
					return fmt.Errorf("unable to get PVC %q: %w", name, err)
				}
				if state.PVC == nil {
					return fmt.Errorf("no PVC %q found: expected existing PVC for DataVolume %q in phase %q", name, state.DV.Name, state.DV.Status.Phase)
				}
			}
		}
	}

	if state.PVC != nil {
		switch state.PVC.Status.Phase {
		case corev1.ClaimBound:
			pvName := state.PVC.Spec.VolumeName
			var err error
			state.PV, err = helper.FetchObject(ctx, types.NamespacedName{Name: pvName, Namespace: state.PVC.Namespace}, client, &corev1.PersistentVolume{})
			if err != nil {
				return fmt.Errorf("unable to get PV %q: %w", pvName, err)
			}
			if state.PV == nil {
				return fmt.Errorf("no PV %q found: expected existing PV for PVC %q in phase %q", pvName, state.PVC.Name, state.PVC.Status.Phase)
			}
		default:
		}
	}

	return state.AttacheeState.Reload(ctx, req, log, client)
}

func (state *VMIReconcilerState) ShouldReconcile(log logr.Logger) bool {
	if state.VMI.IsEmpty() {
		return false
	}

	if state.AttacheeState.ShouldReconcile(log) {
		return true
	}

	return true
}

func (state *VMIReconcilerState) IsProtected() bool {
	return controllerutil.ContainsFinalizer(state.VMI.Current(), virtv2.FinalizerVMICleanup)
}

func (state *VMIReconcilerState) IsReady() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	return state.VMI.Current().Status.Phase == virtv2.ImageReady
}

func (state *VMIReconcilerState) IsDeletion() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	return state.VMI.Current().DeletionTimestamp != nil
}

func (state *VMIReconcilerState) ShouldTrackPod() bool {
	if state.VMI.IsEmpty() {
		return false
	}

	// Always run importer Pod when storage is DVCR.
	if state.VMI.Current().Spec.Storage == virtv2.StorageContainerRegistry {
		return true
	}

	// Run importer Pod for 2 phase import process (HTTP, Upload and ContainerImage sources).
	return vmiutil.IsTwoPhaseImport(state.VMI.Current())
}

// IsPodInited returns whether VMI has annotations with importer or uploader coordinates.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) IsPodInited() bool {
	switch state.VMI.Current().Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		return state.hasUploaderPodAnno()
	default:
		return state.hasImporterPodAnno()
	}
}

// hasImporterPodAnno returns whether VMI has annotations with importer Pod coordinates.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) hasImporterPodAnno() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	anno := state.VMI.Current().GetAnnotations()
	if _, ok := anno[cc.AnnImportPodName]; !ok {
		return false
	}
	return true
}

// hasUploaderPodAnno returns whether VMI has annotations with uploader Pod coordinates.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) hasUploaderPodAnno() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	anno := state.VMI.Current().GetAnnotations()
	if _, ok := anno[cc.AnnUploadPodName]; !ok {
		return false
	}
	return true
}

// CanStartPod returns whether importer/uploader Pod can be started.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) CanStartPod() bool {
	return !state.IsReady() && state.IsPodInited() && state.Pod == nil
}

// IsPodComplete returns whether importer/uploader Pod was completed.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) IsPodComplete() bool {
	return state.Pod != nil && cc.IsPodComplete(state.Pod)
}

// IsPodInProgress returns whether Pod is in progress.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) IsPodInProgress() bool {
	return state.Pod != nil && state.Pod.Status.Phase == corev1.PodRunning
}

func (state *VMIReconcilerState) HasTargetPVCSize() bool {
	return state.GetTargetPVCSize() != ""
}

func (state *VMIReconcilerState) GetTargetPVCSize() string {
	if state.VMI.IsEmpty() {
		return ""
	}

	return state.VMI.Current().Status.Size.UnpackedBytes
}

// ShouldTrackDataVolume returns true if import should be done via DataVolume.
func (state *VMIReconcilerState) ShouldTrackDataVolume() bool {
	if state.VMI.IsEmpty() {
		return false
	}

	return state.VMI.Current().Spec.Storage == virtv2.StorageKubernetes
}

func (state *VMIReconcilerState) HasDataVolumeAnno() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	anno := state.VMI.Current().GetAnnotations()
	_, ok := anno[cc.AnnVMIDataVolume]
	return ok
}

func (state *VMIReconcilerState) CanCreateDataVolume() bool {
	return state.HasDataVolumeAnno() && state.DV == nil
}

func (state *VMIReconcilerState) IsDataVolumeInProgress() bool {
	return state.DV != nil && state.DV.Status.Phase != cdiv1.Succeeded
}

func (state *VMIReconcilerState) IsDataVolumeComplete() bool {
	return state.DV != nil && state.DV.Status.Phase == cdiv1.Succeeded
}
