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
	vmdutil "github.com/deckhouse/virtualization-controller/pkg/common/vmd"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type VMDReconcilerState struct {
	*vmattachee.AttacheeState[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]

	Client      client.Client
	Supplements *supplements.Generator
	Result      *reconcile.Result

	VMD            *helper.Resource[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]
	DV             *cdiv1.DataVolume
	PVC            *corev1.PersistentVolumeClaim
	PV             *corev1.PersistentVolume
	Pod            *corev1.Pod
	Service        *corev1.Service
	Ingress        *netv1.Ingress
	DVCRDataSource *DVCRDataSource
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
		virtv2.DiskDevice,
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
	err := state.VMD.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}

	if state.VMD.IsEmpty() {
		log.Info("Reconcile observe an absent VMD: it may be deleted", "vmd.name", req.Name, "vmd.namespace", req.Namespace)
		return nil
	}

	state.Supplements = &supplements.Generator{
		Prefix:    vmdShortName,
		Name:      state.VMD.Current().Name,
		Namespace: state.VMD.Current().Namespace,
		UID:       state.VMD.Current().UID,
	}

	if state.VMD.Current().Spec.DataSource != nil {
		switch state.VMD.Current().Spec.DataSource.Type {
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
			state.DVCRDataSource, err = NewDVCRDataSourcesForVMD(ctx, state.VMD.Current().Spec.DataSource, state.VMD.Current(), client)
			if err != nil {
				return err
			}
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

	// Importer Pod is not needed if source image is already in DVCR.
	if vmdutil.IsDVCRSource(state.VMD.Current()) {
		return false
	}

	// Use 2 phase import process for HTTP, Upload and ContainerImage sources.
	return vmdutil.IsTwoPhaseImport(state.VMD.Current())
}

// CanStartPod returns whether importer Pod can be started.
// NOTE: valid only if ShouldTrackPod is true.
func (state *VMDReconcilerState) CanStartPod() bool {
	return state.Pod == nil && !state.IsReady()
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

func (state *VMDReconcilerState) CanCreateDataVolume() bool {
	return state.DV == nil && !state.IsReady()
}

func (state *VMDReconcilerState) IsDataVolumeInProgress() bool {
	return state.DV != nil && state.DV.Status.Phase != cdiv1.Succeeded
}

func (state *VMDReconcilerState) IsDataVolumeComplete() bool {
	return state.DV != nil && state.DV.Status.Phase == cdiv1.Succeeded
}
