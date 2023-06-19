package controller

import (
	virtv1 "github.com/deckhouse/virtualization-controller/apis/v1alpha1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconcilerState struct {
	VMD        *virtv1.VirtualMachineDisk
	VMDMutated *virtv1.VirtualMachineDisk
	DV         *cdiv1.DataVolume
}

type VMDReconcilerSyncState struct {
	VMDReconcilerState
	VMDReconcilerSyncResult
}

type VMDReconcilerSyncResult struct {
	Result      *reconcile.Result
	PhaseResult *VMDReconcilerSyncPhaseResult
}

type VMDReconcilerSyncPhaseResult struct {
	Phase virtv1.DiskPhase
	// DVKey *client.ObjectKey
}

type VMDReconcilerUpdateStatusState struct {
	VMDReconcilerState
	Result *reconcile.Result
}
