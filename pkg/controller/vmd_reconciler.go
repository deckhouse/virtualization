package controller

import (
	"context"
	"fmt"
	virtv1 "github.com/deckhouse/virtualization-controller/apis/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconciler struct {
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
	log      logr.Logger
}

func (r *VMDReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("VirtualMachineDisk", req.NamespacedName)
	syncState, err := r.syncState(ctx, log, req)
	return syncState.Result, err
}

func (r *VMDReconciler) syncState(ctx context.Context, log logr.Logger, req reconcile.Request) (VMDSyncState, error) {
	log.Info(fmt.Sprintf("Sync state of %q", req.String()))

	syncState := VMDSyncState{}

	vmd, err := FetchObject[*virtv1.VirtualMachineDisk](ctx, req.NamespacedName, r.client)
	if vmd == nil || err != nil {
		log.Info(fmt.Sprintf("Reconcile observe absent VMD: %s, it may be deleted", req.String()))
		return syncState, err
	}
	syncState.VMD = vmd
	syncState.VMDMutated = vmd.DeepCopy()

	syncState.DV, err = FetchObject[*cdiv1.DataVolume](ctx, req.NamespacedName, r.client)
	if err != nil {
		return syncState, err
	}

	if syncState.DV == nil {
		// DataVolume named after VirtualMachineDisk (?)
		_ = NewDVFromVirtualMachineDisk(req.Namespace, req.Name, syncState.VMD)
	}

	return syncState, nil
}

func NewDVFromVirtualMachineDisk(namespace, name string, vmd *virtv1.VirtualMachineDisk) *cdiv1.DataVolume {
	labels := map[string]string{}
	annotations := map[string]string{}

	// FIXME: resource.Quantity should be defined directly in the spec struct (see PVC impl. for details)
	pvcSize, err := resource.ParseQuantity(vmd.Spec.PersistentVolumeClaim.Size)
	if err != nil {
		panic(err.Error())
	}

	res := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{},
			PVC: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: &vmd.Spec.PersistentVolumeClaim.StorageClassName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: pvcSize,
					},
				},
			},
		},
	}

	if vmd.Spec.DataSource.HTTP != nil {
		res.Spec.Source.HTTP = &cdiv1.DataVolumeSourceHTTP{
			URL: vmd.Spec.DataSource.HTTP.URL,
		}
	}

	return res
}
