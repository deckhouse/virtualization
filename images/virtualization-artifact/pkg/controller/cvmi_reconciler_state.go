package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CVMIReconcilerState struct {
	*vmattachee.AttacheeState[*virtv2.ClusterVirtualImage, virtv2.ClusterVirtualImageStatus]

	Client      client.Client
	Supplements *supplements.Generator
	Result      *reconcile.Result
	Namespace   string

	CVMI           *helper.Resource[*virtv2.ClusterVirtualImage, virtv2.ClusterVirtualImageStatus]
	Service        *corev1.Service
	Ingress        *netv1.Ingress
	Pod            *corev1.Pod
	DVCRDataSource *DVCRDataSource
}

func NewCVMIReconcilerState(controllerNamespace string) func(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *CVMIReconcilerState {
	return func(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *CVMIReconcilerState {
		state := &CVMIReconcilerState{
			Client: client,
			CVMI: helper.NewResource(
				name, log, client, cache,
				func() *virtv2.ClusterVirtualImage { return &virtv2.ClusterVirtualImage{} },
				func(obj *virtv2.ClusterVirtualImage) virtv2.ClusterVirtualImageStatus {
					return obj.Status
				},
			),
			Namespace: controllerNamespace,
		}
		state.AttacheeState = vmattachee.NewAttacheeState(
			state,
			virtv2.FinalizerCVMIProtection,
			state.CVMI,
		)
		return state
	}
}

func (state *CVMIReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.CVMI.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update CVI %q meta: %w", state.CVMI.Name(), err)
	}
	return nil
}

func (state *CVMIReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.CVMI.UpdateStatus(ctx)
}

func (state *CVMIReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *CVMIReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *CVMIReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	err := state.CVMI.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.CVMI.IsEmpty() {
		log.Info("Reconcile observe an absent CVI: it may be deleted", "cvi", req.NamespacedName)
		return nil
	}

	state.Supplements = &supplements.Generator{
		Prefix:    cvmiShortName,
		Name:      state.CVMI.Current().Name,
		Namespace: state.Namespace,
		UID:       state.CVMI.Current().UID,
	}

	ds := state.CVMI.Current().Spec.DataSource

	switch ds.Type {
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
		state.DVCRDataSource, err = NewDVCRDataSourcesForCVMI(ctx, state.CVMI.Current().Spec.DataSource, client)
		if err != nil {
			return err
		}
	}

	return state.AttacheeState.Reload(ctx, req, log, client)
}

// ShouldReconcile tells if Sync and UpdateStatus should run.
func (state *CVMIReconcilerState) ShouldReconcile(log logr.Logger) bool {
	// CVMI was not found. E.g. CVMI was deleted, but requeue task was triggered.
	if state.CVMI.IsEmpty() {
		return false
	}
	if state.AttacheeState.ShouldReconcile(log) {
		return true
	}
	return true
}

func (state *CVMIReconcilerState) IsFailed() bool {
	if state.CVMI.IsEmpty() {
		return false
	}
	return state.CVMI.Current().Status.Phase == virtv2.ImageFailed
}

func (state *CVMIReconcilerState) IsProtected() bool {
	return controllerutil.ContainsFinalizer(state.CVMI.Current(), virtv2.FinalizerCVMICleanup)
}

func (state *CVMIReconcilerState) IsDeletion() bool {
	if state.CVMI.IsEmpty() {
		return false
	}
	return state.CVMI.Current().DeletionTimestamp != nil
}

func (state *CVMIReconcilerState) IsImportInProgress() bool {
	return state.Pod != nil && state.Pod.Status.Phase == corev1.PodRunning
}

func (state *CVMIReconcilerState) IsImportInPending() bool {
	return state.Pod != nil && state.Pod.Status.Phase == corev1.PodPending
}

// CanStartPod returns whether importer Pod can be started.
// NOTE: valid only if ShouldTrackPod is true.
func (state *CVMIReconcilerState) CanStartPod() bool {
	return !state.IsReady() && !state.IsFailed() && state.Pod == nil
}

func (state *CVMIReconcilerState) IsReady() bool {
	if state.CVMI.IsEmpty() {
		return false
	}
	return state.CVMI.Current().Status.Phase == virtv2.ImageReady
}

// IsPodComplete returns whether importer/uploader Pod was completed.
// NOTE: valid only if ShouldTrackPod is true.
func (state *CVMIReconcilerState) IsPodComplete() bool {
	return state.Pod != nil && cc.IsPodComplete(state.Pod)
}

func (state *CVMIReconcilerState) IsAttachedToVM(vm virtv2.VirtualMachine) bool {
	if state.CVMI.IsEmpty() {
		return false
	}

	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.ClusterImageDevice && bda.Name == state.CVMI.Name().Name {
			return true
		}
	}

	return false
}
