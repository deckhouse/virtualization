package vmattachee

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type AttacheeState[T helper.Object[T, ST], ST any] struct {
	two_phase_reconciler.ReconcilerState

	Kind                string
	ProtectionFinalizer string
	AttachedVMs         []*virtv2.VirtualMachine
	Resource            *helper.Resource[T, ST]
}

func NewAttacheeState[T helper.Object[T, ST], ST any](reconcilerState two_phase_reconciler.ReconcilerState, kind, protectionFinalizer string, resource *helper.Resource[T, ST]) *AttacheeState[T, ST] {
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
	attachedVMs, err := state.findAttachedVMs(ctx, client)
	if err != nil {
		return err
	}
	state.AttachedVMs = attachedVMs

	log.V(2).Info("Attachee Reload", "Kind", state.Kind, "AttachedVMs", attachedVMs)

	return nil
}

func (state *AttacheeState[T, ST]) findAttachedVMs(ctx context.Context, c client.Client) ([]*virtv2.VirtualMachine, error) {
	vml := &virtv2.VirtualMachineList{}

	req, err := labels.NewRequirement(
		MakeAttachedResourceLabelKeyFormat(state.Kind, state.Resource.Name().Name),
		selection.Equals,
		[]string{AttachedLabelValue},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create label requirement: %w", err)
	}

	sel := labels.NewSelector()
	sel.Add(*req)

	if err := c.List(ctx, vml, &client.ListOptions{LabelSelector: sel}); err != nil {
		return nil, fmt.Errorf("error getting VM by selector %v: %w", sel, err)
	}
	return util.ToPointersArray(vml.Items), nil
}

func (state *AttacheeState[T, ST]) ShouldRemoveProtectionFinalizer() bool {
	return controllerutil.ContainsFinalizer(state.Resource.Current(), state.ProtectionFinalizer) && (len(state.AttachedVMs) == 0)
}

func (state *AttacheeState[T, ST]) RemoveProtectionFinalizer() {
	controllerutil.RemoveFinalizer(state.Resource.Changed(), state.ProtectionFinalizer)
}
