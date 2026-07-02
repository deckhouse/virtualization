//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package vmpool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ScaleWebhookPath is where the scale-subresource guard is served. It must match
// the ValidatingWebhookConfiguration entry for virtualmachinepools/scale.
const ScaleWebhookPath = "/validate-virtualization-deckhouse-io-v1alpha2-virtualmachinepool-scale"

// SetupScaleWebhook registers the guard that rejects anonymous scale-down via
// the scale subresource for pools with scaleDownPolicy: Explicit.
func SetupScaleWebhook(mgr manager.Manager) {
	// Gated like the controller: while the feature gate is off the guard is not
	// registered (the CRD's scale subresource is still served, just unguarded —
	// there is no controller to act on it either).
	if !featuregates.Default().Enabled(featuregates.VirtualMachinePool) {
		return
	}
	mgr.GetWebhookServer().Register(ScaleWebhookPath, &webhook.Admission{
		Handler: &scaleValidator{client: mgr.GetClient()},
	})
}

type scaleValidator struct {
	client client.Client
}

func (v *scaleValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Only UPDATE of the scale subresource carries a replicas change to guard.
	if req.SubResource != "scale" || req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}

	var newScale, oldScale autoscalingv1.Scale
	if err := json.Unmarshal(req.Object.Raw, &newScale); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode new Scale: %w", err))
	}
	if err := json.Unmarshal(req.OldObject.Raw, &oldScale); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode old Scale: %w", err))
	}

	// Only a decrease is anonymous scale-down; growth and no-ops are always fine.
	if newScale.Spec.Replicas >= oldScale.Spec.Replicas {
		return admission.Allowed("")
	}

	pool := &v1alpha2.VirtualMachinePool{}
	if err := v.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, pool); err != nil {
		if apierrors.IsNotFound(err) {
			return admission.Allowed("")
		}
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get VirtualMachinePool %s/%s: %w", req.Namespace, req.Name, err))
	}

	if pool.Spec.ScaleDownPolicy == v1alpha2.ScaleDownPolicyExplicit {
		return admission.Denied(fmt.Sprintf(
			"VirtualMachinePool %q uses scaleDownPolicy Explicit: decreasing replicas through the scale subresource is not allowed. "+
				"Remove specific virtual machines with the scaleDownWith subresource instead.",
			req.Name,
		))
	}

	return admission.Allowed("")
}
