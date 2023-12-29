package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
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
	Service        *corev1.Service
	Ingress        *netv1.Ingress
	DVCRDataSource *DVCRDataSource
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
		virtv2.ImageDevice,
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
	err := state.VMI.Fetch(ctx)
	if err != nil {
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
		state.Pod, err = uploader.FindPod(ctx, client, state.Supplements)
		if err != nil {
			return err
		}

		state.Service, err = uploader.FindService(ctx, client, state.Supplements)
		if err != nil {
			return err
		}

		state.Ingress, err = uploader.FindIngress(ctx, client, state.Supplements)
		if err != nil {
			return err
		}
	default:
		state.Pod, err = importer.FindPod(ctx, client, state.Supplements)
		if err != nil {
			return err
		}

		// TODO These resources are not part of the state. Retrieve additional resources in Sync phase.
		state.DVCRDataSource, err = NewDVCRDataSourcesForVMI(ctx, state.VMI.Current().Spec.DataSource, state.VMI.Current(), client)
		if err != nil {
			return err
		}
	}

	dvName := state.Supplements.DataVolume()
	state.DV, err = helper.FetchObject(ctx, dvName, client, &cdiv1.DataVolume{})
	if err != nil {
		return fmt.Errorf("unable to get DV %q: %w", dvName, err)
	}

	pvcName := dvName
	state.PVC, err = helper.FetchObject(ctx, pvcName, client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("unable to get PVC %q: %w", pvcName, err)
	}

	if state.PVC != nil && state.PVC.Status.Phase == corev1.ClaimBound {
		pvName := types.NamespacedName{Name: state.PVC.Spec.VolumeName, Namespace: state.PVC.Namespace}
		state.PV, err = helper.FetchObject(ctx, pvName, client, &corev1.PersistentVolume{})
		if err != nil {
			return fmt.Errorf("unable to get PV %q: %w", pvName, err)
		}
		if state.PV == nil {
			return fmt.Errorf("no PV %q found: expected existing PV for PVC %q in phase %q", pvName, state.PVC.Name, state.PVC.Status.Phase)
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

func (state *VMIReconcilerState) IsLost() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	return state.VMI.Current().Status.Phase == virtv2.ImagePVCLost
}

func (state *VMIReconcilerState) IsReady() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	return state.VMI.Current().Status.Phase == virtv2.ImageReady
}

func (state *VMIReconcilerState) IsFailed() bool {
	if state.VMI.IsEmpty() {
		return false
	}
	return state.VMI.Current().Status.Phase == virtv2.ImageFailed
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

// CanStartPod returns whether importer/uploader Pod can be started.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMIReconcilerState) CanStartPod() bool {
	return !state.IsReady() && !state.IsFailed() && state.Pod == nil
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

func (state *VMIReconcilerState) CanCreateDataVolume() bool {
	return state.DV == nil && !state.IsReady()
}

func (state *VMIReconcilerState) IsDataVolumeInProgress() bool {
	return state.DV != nil && state.DV.Status.Phase != cdiv1.Succeeded
}

func (state *VMIReconcilerState) IsDataVolumeComplete() bool {
	return state.DV != nil && state.DV.Status.Phase == cdiv1.Succeeded
}
