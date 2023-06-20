package controller

import (
	"context"
	"fmt"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CVMIReconciler struct {
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
	log      logr.Logger
}

// Reconcile loop for CVMIReconciler.
func (r *CVMIReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	cvmImage := &virtv2.ClusterVirtualMachineImage{}
	if err := r.client.Get(ctx, req.NamespacedName, cvmImage); err != nil {
		if k8serrors.IsNotFound(err) {
			r.log.Info(fmt.Sprintf("Reconcile observe absent CVMI: %s, it may be deleted", req.String()))
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	// Use cvmImage to start builder Pod.
	r.log.Info(fmt.Sprintf("Reconcile for CVMI: %s", req.String()))

	return reconcile.Result{}, nil
}
