package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

var ErrImagePullSecretNotFound = errors.New("imagePullSecret not found")

type CVMIReconcilerState struct {
	*vmattachee.AttacheeState[*virtv2.ClusterVirtualMachineImage, virtv2.ClusterVirtualMachineImageStatus]

	Client          client.Client
	CVMI            *helper.Resource[*virtv2.ClusterVirtualMachineImage, virtv2.ClusterVirtualMachineImageStatus]
	Service         *corev1.Service
	Pod             *corev1.Pod
	ImagePullSecret *ImagePullSecret
	AttachedVMs     []*virtv2.VirtualMachine
	Result          *reconcile.Result
}

type ImagePullSecret struct {
	Secret       *corev1.Secret
	SourceSecret *corev1.Secret
}

func NewCVMIReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *CVMIReconcilerState {
	state := &CVMIReconcilerState{
		Client: client,
		CVMI: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.ClusterVirtualMachineImage { return &virtv2.ClusterVirtualMachineImage{} },
			func(obj *virtv2.ClusterVirtualMachineImage) virtv2.ClusterVirtualMachineImageStatus {
				return obj.Status
			},
		),
	}
	state.AttacheeState = vmattachee.NewAttacheeState(
		state,
		"cvmi",
		virtv2.FinalizerCVMIProtection,
		state.CVMI,
	)
	return state
}

func (state *CVMIReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.CVMI.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update CVMI %q meta: %w", state.CVMI.Name(), err)
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
	if err := state.CVMI.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.CVMI.IsEmpty() {
		log.Info("Reconcile observe an absent CVMI: it may be deleted", "CVMI", req.NamespacedName)
		return nil
	}

	switch state.CVMI.Current().Spec.DataSource.Type {
	case virtv2.DataSourceTypeUpload:
		pod, err := uploader.FindPod(ctx, client, state.CVMI.Current())
		if err != nil && !errors.Is(err, uploader.ErrPodNameNotFound) {
			return err
		}
		state.Pod = pod

		service, err := uploader.FindService(ctx, client, state.CVMI.Current())
		if err != nil && !errors.Is(err, uploader.ErrServiceNameNotFound) {
			return err
		}
		state.Service = service
	default:
		pod, err := importer.FindPod(ctx, client, state.CVMI.Current())
		if err != nil && !errors.Is(err, importer.ErrPodNameNotFound) {
			return err
		}

		secretName := state.CVMI.Current().Spec.DataSource.ContainerImage.ImagePullSecret.Name
		secretNS := state.CVMI.Current().Spec.DataSource.ContainerImage.ImagePullSecret.Namespace
		if secretName != "" && secretNS != "" {
			secret, err := importer.FindSecret(ctx, client, state.CVMI.Current())
			if err != nil && !errors.Is(err, importer.ErrSecretNameNotFound) {
				return err
			}
			imgPullSecret := &ImagePullSecret{Secret: secret}
			if !errors.Is(err, importer.ErrSecretNameNotFound) {
				srcSecret, err := helper.FetchObject[*corev1.Secret](ctx, types.NamespacedName{Name: secretName, Namespace: secretNS}, client, &corev1.Secret{})
				if err != nil {
					return err
				}
				if srcSecret == nil {
					return ErrImagePullSecretNotFound
				}
				imgPullSecret.SourceSecret = srcSecret
				state.ImagePullSecret = imgPullSecret
			}
		}
		state.Pod = pod
	}

	return state.AttacheeState.Reload(ctx, req, log, client)
}

// ShouldReconcile tells if Sync and UpdateStatus should run.
// CVMI should not be reconciled if phase is Ready and no importer or uploader Pod found.
func (state *CVMIReconcilerState) ShouldReconcile(log logr.Logger) bool {
	// CVMI was not found. E.g. CVMI was deleted, but requeue task was triggered.
	if state.CVMI.IsEmpty() {
		return false
	}
	if state.AttacheeState.ShouldReconcile(log) {
		return true
	}
	return !(cc.IsCVMIComplete(state.CVMI.Current()) && state.Pod == nil && state.Service == nil)
}

func (state *CVMIReconcilerState) IsProtected() bool {
	return controllerutil.ContainsFinalizer(state.CVMI.Current(), virtv2.FinalizerCVMICleanup)
}

// HasImporterPodAnno returns whether CVMI is just created and has no annotations with importer Pod coordinates.
func (state *CVMIReconcilerState) HasImporterAnno() bool {
	if state.CVMI.IsEmpty() {
		return false
	}
	cvmi := state.CVMI.Current()
	anno := cvmi.GetAnnotations()
	if _, ok := anno[cc.AnnImportPodName]; !ok {
		return false
	}
	if _, ok := anno[cc.AnnImporterNamespace]; !ok {
		return false
	}

	if cvmi.Spec.DataSource.Type == virtv2.DataSourceTypeContainerImage &&
		cvmi.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" &&
		cvmi.Spec.DataSource.ContainerImage.ImagePullSecret.Namespace != "" {
		if _, ok := anno[cc.AnnAuthSecret]; !ok {
			return false
		}
	}
	return true
}

// HasUploaderAnno returns whether CVMI is just created and has no annotations with uploader Pod or Service coordinates.
func (state *CVMIReconcilerState) HasUploaderAnno() bool {
	if state.CVMI.IsEmpty() {
		return false
	}
	anno := state.CVMI.Current().GetAnnotations()
	if _, ok := anno[cc.AnnUploaderNamespace]; !ok {
		return false
	}
	if _, ok := anno[cc.AnnUploadPodName]; !ok {
		return false
	}
	if _, ok := anno[cc.AnnUploadServiceName]; !ok {
		return false
	}
	return true
}

func (state *CVMIReconcilerState) IsDeletion() bool {
	if state.CVMI.IsEmpty() {
		return false
	}
	return state.CVMI.Current().DeletionTimestamp != nil
}

func (state *CVMIReconcilerState) IsImportComplete() bool {
	return state.Pod != nil && cc.IsPodComplete(state.Pod)
}

func (state *CVMIReconcilerState) IsImportInProgress() bool {
	return state.Pod != nil && state.Pod.Status.Phase == corev1.PodRunning
}
