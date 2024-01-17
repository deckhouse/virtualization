package vmattachee

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type AttacheeState[T helper.Object[T, ST], ST any] struct {
	two_phase_reconciler.ReconcilerState

	Kind                virtv2.BlockDeviceType
	ProtectionFinalizer string
	isAttached          bool
	Resource            *helper.Resource[T, ST]
}

func NewAttacheeState[T helper.Object[T, ST], ST any](reconcilerState two_phase_reconciler.ReconcilerState, kind virtv2.BlockDeviceType, protectionFinalizer string, resource *helper.Resource[T, ST]) *AttacheeState[T, ST] {
	return &AttacheeState[T, ST]{
		ReconcilerState:     reconcilerState,
		Kind:                kind,
		ProtectionFinalizer: protectionFinalizer,
		Resource:            resource,
	}
}

func (state *AttacheeState[T, ST]) ShouldReconcile(log logr.Logger) bool {
	log.V(2).Info("AttacheeState ShouldReconcile", "Kind", state.Kind, "ShouldRemoveProtectionFinalizer", state.ShouldRemoveProtectionFinalizer())
	return state.ShouldRemoveProtectionFinalizer()
}

func (state *AttacheeState[T, ST]) Reload(ctx context.Context, _ reconcile.Request, log logr.Logger, client client.Client) error {
	isAttached, err := state.hasAttachedVM(ctx, client)
	if err != nil {
		return err
	}
	state.isAttached = isAttached

	log.V(2).Info("Attachee Reload", "Kind", state.Kind, "isAttached", state.isAttached)

	return nil
}

func (state *AttacheeState[T, ST]) hasAttachedVM(ctx context.Context, apiClient client.Client) (bool, error) {
	var vms virtv2.VirtualMachineList
	err := apiClient.List(ctx, &vms, &client.ListOptions{
		Namespace: state.Resource.Name().Namespace,
	})
	if err != nil {
		return false, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		for _, bda := range vm.Status.BlockDevicesAttached {
			switch state.Kind {
			case virtv2.ClusterImageDevice:
				if state.Kind == bda.Type && bda.ClusterVirtualMachineImage != nil && bda.ClusterVirtualMachineImage.Name == state.Resource.Name().Name {
					return true, nil
				}
			case virtv2.ImageDevice:
				if state.Kind == bda.Type && bda.VirtualMachineImage != nil && bda.VirtualMachineImage.Name == state.Resource.Name().Name {
					return true, nil
				}
			case virtv2.DiskDevice:
				if state.Kind == bda.Type && bda.VirtualMachineDisk != nil && bda.VirtualMachineDisk.Name == state.Resource.Name().Name {
					return true, nil
				}
			default:
				return false, fmt.Errorf("unexpected block device kind: %s", state.Kind)
			}
		}
	}

	return false, nil
}

func (state *AttacheeState[T, ST]) ShouldRemoveProtectionFinalizer() bool {
	return controllerutil.ContainsFinalizer(state.Resource.Current(), state.ProtectionFinalizer) && !state.isAttached
}

func (state *AttacheeState[T, ST]) RemoveProtectionFinalizer() {
	controllerutil.RemoveFinalizer(state.Resource.Changed(), state.ProtectionFinalizer)
}
