package controller

import (
	virtv1 "github.com/deckhouse/virtualization-controller/apis/v1alpha1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDSyncState struct {
	VMD        *virtv1.VirtualMachineDisk
	VMDMutated *virtv1.VirtualMachineDisk
	DV         *cdiv1.DataVolume

	VMDSyncResult
}

type VMDSyncResult struct {
	Result    reconcile.Result
	PhaseSync *VMDStatusPhase
}

type VMDStatusPhase struct {
	Phase virtv1.VirtualMachineDiskPhase
	DVKey *client.ObjectKey
}
