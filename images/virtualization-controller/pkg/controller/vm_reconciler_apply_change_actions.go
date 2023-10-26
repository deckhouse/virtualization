package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

func (r *VMReconciler) ApplyChangesWithRestart(ctx context.Context, action *kvbuilder.ChangeApplyActions, state *VMReconcilerState, kvvm *virtv1.VirtualMachine, opts two_phase_reconciler.ReconcilerOptions) error {
	message := fmt.Sprintf("VM restart required on changes: %s", strings.Join(action.GetChangesTitles(), ", "))
	opts.Recorder.Event(state.VM.Current(), corev1.EventTypeNormal, "RestartVMToApplyChanges", message)

	if err := opts.Client.Update(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to update KubeVirt VM %q: %w", kvvm.Name, err)
	}
	state.KVVM = kvvm

	if err := opts.Client.Delete(ctx, state.KVVMI); err != nil {
		return fmt.Errorf("unable to remove current KubeVirt VMI %q: %w", state.KVVMI.Name, err)
	}
	state.KVVMI = nil

	return nil
}
