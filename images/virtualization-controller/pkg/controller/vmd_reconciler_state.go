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
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type VMDReconcilerState struct {
	*vmattachee.AttacheeState[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]

	Client  client.Client
	VMD     *helper.Resource[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]
	DV      *cdiv1.DataVolume
	PVC     *corev1.PersistentVolumeClaim
	PV      *corev1.PersistentVolume
	Pod     *corev1.Pod
	Service *corev1.Service
	Result  *reconcile.Result
}

func NewVMDReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMDReconcilerState {
	state := &VMDReconcilerState{
		Client: client,
		VMD: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineDisk { return &virtv2.VirtualMachineDisk{} },
			func(obj *virtv2.VirtualMachineDisk) virtv2.VirtualMachineDiskStatus { return obj.Status },
		),
	}

	state.AttacheeState = vmattachee.NewAttacheeState(
		state,
		"vmd",
		virtv2.FinalizerVMDProtection,
		state.VMD,
	)

	return state
}

func (state *VMDReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VMD.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMD %q meta: %w", state.VMD.Name(), err)
	}
	return nil
}

func (state *VMDReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.VMD.UpdateStatus(ctx)
}

func (state *VMDReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMDReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMDReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	if err := state.VMD.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.VMD.IsEmpty() {
		log.Info("Reconcile observe an absent VMD: it may be deleted", "vmd.name", req.Name, "vmd.namespace", req.Namespace)
		return nil
	}

	if state.VMD.Current().Spec.DataSource != nil {
		switch state.VMD.Current().Spec.DataSource.Type {
		case virtv2.DataSourceTypeUpload:
			pod, err := uploader.FindPod(ctx, client, state.VMD.Current())
			if err != nil && !errors.Is(err, uploader.ErrPodNameNotFound) {
				return err
			}
			state.Pod = pod

			service, err := uploader.FindService(ctx, client, state.VMD.Current())
			if err != nil && !errors.Is(err, uploader.ErrServiceNameNotFound) {
				return err
			}
			state.Service = service
		default:
			pod, err := importer.FindPod(ctx, client, state.VMD.Current())
			if err != nil && !errors.Is(err, importer.ErrPodNameNotFound) {
				return err
			}
			state.Pod = pod
		}
	}

	if dvName, hasKey := state.VMD.Current().Annotations[cc.AnnVMDDataVolume]; hasKey {
		var err error
		name := types.NamespacedName{Name: dvName, Namespace: state.VMD.Current().Namespace}

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

func (state *VMDReconcilerState) ShouldReconcile(log logr.Logger) bool {
	if state.VMD.IsEmpty() {
		return false
	}

	if state.AttacheeState.ShouldReconcile(log) {
		return true
	}

	return true
}

func (state *VMDReconcilerState) IsProtected() bool {
	return controllerutil.ContainsFinalizer(state.VMD.Current(), virtv2.FinalizerVMDCleanup)
}

func (state *VMDReconcilerState) IsReady() bool {
	if state.VMD.IsEmpty() {
		return false
	}
	return state.VMD.Current().Status.Phase == virtv2.DiskReady
}

func (state *VMDReconcilerState) IsDeletion() bool {
	if state.VMD.IsEmpty() {
		return false
	}
	return state.VMD.Current().DeletionTimestamp != nil
}

func (state *VMDReconcilerState) ShouldTrackPod() bool {
	if state.VMD.IsEmpty() {
		return false
	}

	if state.VMD.Current().Spec.DataSource == nil {
		return false
	}

	// Use 2 phase import process for HTTP, Upload and ContainerImage sources.
	switch state.VMD.Current().Spec.DataSource.Type {
	case virtv2.DataSourceTypeHTTP,
		virtv2.DataSourceTypeUpload,
		virtv2.DataSourceTypeContainerImage:
		return true
	}

	return false
}

// IsPodInited returns whether VMD has annotations with importer or uploader coordinates.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMDReconcilerState) IsPodInited() bool {
	if state.VMD.Current().Spec.DataSource == nil {
		return false
	}

	switch state.VMD.Current().Spec.DataSource.Type {
	case virtv2.DataSourceTypeHTTP:
		return state.hasImporterPodAnno()
	case virtv2.DataSourceTypeUpload:
		return state.hasUploaderPodAnno()
	default:
		return false
	}
}

// hasImporterPodAnno returns whether VMD has annotations with importer Pod coordinates.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMDReconcilerState) hasImporterPodAnno() bool {
	if state.VMD.IsEmpty() {
		return false
	}
	anno := state.VMD.Current().GetAnnotations()
	if _, ok := anno[cc.AnnImportPodName]; !ok {
		return false
	}
	return true
}

// hasImporterPodAnno returns whether VMD has annotations with uploader Pod coordinates.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMDReconcilerState) hasUploaderPodAnno() bool {
	if state.VMD.IsEmpty() {
		return false
	}
	anno := state.VMD.Current().GetAnnotations()
	if _, ok := anno[cc.AnnUploadPodName]; !ok {
		return false
	}
	return true
}

// CanStartPod returns whether importer Pod can be started.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMDReconcilerState) CanStartPod() bool {
	return !state.IsReady() && state.IsPodInited() && state.Pod == nil
}

// IsPodComplete returns whether importer/uploader Pod was completed.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMDReconcilerState) IsPodComplete() bool {
	return state.Pod != nil && cc.IsPodComplete(state.Pod)
}

// IsPodRunning returns whether importer/uploader Pod is in progress.
func (state *VMDReconcilerState) IsPodRunning() bool {
	return state.Pod != nil && state.Pod.Status.Phase == corev1.PodRunning
}

func (state *VMDReconcilerState) ShouldTrackDataVolume() bool {
	return !state.VMD.IsEmpty()
}

func (state *VMDReconcilerState) HasDataVolumeAnno() bool {
	if state.VMD.IsEmpty() {
		return false
	}
	anno := state.VMD.Current().GetAnnotations()
	_, ok := anno[cc.AnnVMDDataVolume]
	return ok
}

func (state *VMDReconcilerState) CanCreateDataVolume() bool {
	return state.HasDataVolumeAnno() && state.DV == nil
}

func (state *VMDReconcilerState) IsDataVolumeInProgress() bool {
	return state.DV != nil && state.DV.Status.Phase != cdiv1.Succeeded
}

func (state *VMDReconcilerState) IsDataVolumeComplete() bool {
	return state.DV != nil && state.DV.Status.Phase == cdiv1.Succeeded
}
