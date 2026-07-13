/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const nameAutoCoreFractionHandler = "AutoCoreFractionHandler"

// AutoCoreFractionHandler drives the coreFraction of VMs opted into vertical
// autoscaling (spec.cpu.coreFraction == "auto"). It owns one VPA per such VM
// (updateMode Off, so the recommender never evicts the pod), seeds
// status.autoCoreFraction on first sight, and moves it to the value derived from the
// VPA recommendation. SyncKvvm resolves "auto" to that number and applies it in
// place. There is no cross-VM orchestration: a VM that leaves the recommended range
// is moved, up or down.
type AutoCoreFractionHandler struct {
	client         client.Client
	recorder       eventrecord.EventRecorderLogger
	scheme         *runtime.Scheme
	coreFractioner *service.CoreFractionService
	featureGate    featuregate.FeatureGate
}

func NewAutoCoreFractionHandler(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	scheme *runtime.Scheme,
	coreFractioner *service.CoreFractionService,
	featureGate featuregate.FeatureGate,
) *AutoCoreFractionHandler {
	return &AutoCoreFractionHandler{
		client:         client,
		recorder:       recorder,
		scheme:         scheme,
		coreFractioner: coreFractioner,
		featureGate:    featureGate,
	}
}

func (h *AutoCoreFractionHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	// The gate implies the VPA CRD is installed and in-place hotplug is on. The webhook
	// already blocks coreFraction "auto" without it, so this is defence in depth that
	// also keeps the handler quiet where the VPA CRD is absent.
	if !h.enabled() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Changed()

	if vm.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, h.deleteVPA(ctx, vm)
	}

	// Autoscaling is opt-in per VM. When off, drop the VPA and retract the driven value;
	// a VM that was never autoscaled carries none, so skip to avoid a delete per reconcile.
	if vm.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		if vm.Status.AutoCoreFraction == "" {
			return reconcile.Result{}, nil
		}
		if err := h.deleteVPA(ctx, vm); err != nil {
			return reconcile.Result{}, err
		}
		vm.Status.AutoCoreFraction = ""
		return reconcile.Result{}, nil
	}

	if err := h.ensureVPA(ctx, vm); err != nil {
		return reconcile.Result{}, err
	}

	class, err := s.Class(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if class == nil {
		return reconcile.Result{}, nil
	}

	// Seed with a middle Burstable value so the VM starts inside the autoscaling range.
	if vm.Status.AutoCoreFraction == "" {
		vm.Status.AutoCoreFraction = fmt.Sprintf("%d%%", sizingpolicy.SeedAutoCoreFraction(class, vm.Spec.CPU.Cores))
		return reconcile.Result{}, nil
	}

	vpaObj := &vpav1.VerticalPodAutoscaler{}
	if err := h.client.Get(ctx, client.ObjectKeyFromObject(vm), vpaObj); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("get VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), vm.GetName(), err)
	}

	if override, ok, err := recommendationOverride(vpaObj); err != nil {
		logger.FromContext(ctx).Warn("Ignoring malformed recommendation override annotation", logger.SlogErr(err))
	} else if ok {
		vpaObj.Status.Recommendation = override
	}

	rec, ok := computeCPURecommendation(vpaObj)
	if !ok {
		return reconcile.Result{}, nil
	}

	decision, err := h.coreFractioner.Calculate(vm, class, rec)
	if err != nil {
		return reconcile.Result{}, err
	}
	if decision.Direction == service.DirectionNone {
		return reconcile.Result{}, nil
	}

	desired := fmt.Sprintf("%d%%", decision.DesiredCoreFraction)
	if desired == vm.Status.AutoCoreFraction {
		return reconcile.Result{}, nil
	}

	logger.FromContext(ctx).Info("Updating desired coreFraction from VPA recommendation",
		slog.String("direction", decision.Direction.String()),
		slog.String("from", vm.Status.AutoCoreFraction),
		slog.String("to", desired),
	)
	h.recorder.Eventf(vm, corev1.EventTypeNormal, v1alpha2.ReasonCoreFractionScaling,
		"Scaling CPU core fraction %s from %s to %s.", decision.Direction, vm.Status.AutoCoreFraction, desired)
	vm.Status.AutoCoreFraction = desired

	return reconcile.Result{}, nil
}

func (h *AutoCoreFractionHandler) Name() string {
	return nameAutoCoreFractionHandler
}

func (h *AutoCoreFractionHandler) enabled() bool {
	return h.featureGate.Enabled(featuregates.VerticalVirtualMachineAutoscaler) &&
		h.featureGate.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize)
}

func (h *AutoCoreFractionHandler) ensureVPA(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	existing := &vpav1.VerticalPodAutoscaler{}
	err := h.client.Get(ctx, client.ObjectKeyFromObject(vm), existing)
	if err == nil {
		// Create-only on purpose: the handler never patches an existing VPA, so a
		// recommendation-override annotation on it (see annotationRecommendationOverride)
		// is never stripped. Keep it create-only if this ever grows an update path.
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), vm.GetName(), err)
	}

	desired := newVPAForVirtualMachine(vm)
	if err := controllerutil.SetControllerReference(vm, desired, h.scheme); err != nil {
		return fmt.Errorf("set owner reference on VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), vm.GetName(), err)
	}

	if err := h.client.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), vm.GetName(), err)
	}
	return nil
}

func (h *AutoCoreFractionHandler) deleteVPA(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	obj := &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: vm.GetName(), Namespace: vm.GetNamespace()},
	}
	if err := h.client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), vm.GetName(), err)
	}
	return nil
}

// newVPAForVirtualMachine builds the VPA for a VM in updateMode Off. targetRef points
// at the subresources API group, not the core CRD group: the /scale subresource is
// served there by the aggregated apiserver, and the recommender only treats a
// targetRef as scalable when its group/kind exposes /scale in discovery.
func newVPAForVirtualMachine(vm *v1alpha2.VirtualMachine) *vpav1.VerticalPodAutoscaler {
	return &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.GetName(),
			Namespace: vm.GetNamespace(),
		},
		Spec: vpav1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscalingv1.CrossVersionObjectReference{
				APIVersion: subv1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
				Name:       vm.GetName(),
			},
			UpdatePolicy: &vpav1.PodUpdatePolicy{
				UpdateMode: ptr.To(vpav1.UpdateModeOff),
			},
		},
	}
}

// recommendationOverride returns the recommendation pinned via
// [annotationons.AnnRecommendationOverride, if the annotation is present and well-formed.
// A malformed value returns an error so the caller can log and fall back to the
// recommender's own status rather than break the reconcile.
func recommendationOverride(vpaObj *vpav1.VerticalPodAutoscaler) (*vpav1.RecommendedPodResources, bool, error) {
	raw, ok := vpaObj.GetAnnotations()[annotations.AnnRecommendationOverride]
	if !ok || raw == "" {
		return nil, false, nil
	}
	override := &vpav1.RecommendedPodResources{}
	if err := json.Unmarshal([]byte(raw), override); err != nil {
		return nil, false, fmt.Errorf("unmarshal %s annotation: %w", annotations.AnnRecommendationOverride, err)
	}
	return override, true, nil
}

// computeCPURecommendation extracts the CPU target and bounds (millicores) for the
// compute container, if the VPA has a recommendation for it.
func computeCPURecommendation(vpaObj *vpav1.VerticalPodAutoscaler) (service.Recommendation, bool) {
	if vpaObj.Status.Recommendation == nil {
		return service.Recommendation{}, false
	}
	for _, cr := range vpaObj.Status.Recommendation.ContainerRecommendations {
		if !commonvm.IsComputeContainer(cr.ContainerName) {
			continue
		}
		target, ok := cr.Target[corev1.ResourceCPU]
		if !ok {
			return service.Recommendation{}, false
		}
		rec := service.Recommendation{TargetMilli: target.MilliValue()}
		if lb, ok := cr.LowerBound[corev1.ResourceCPU]; ok {
			rec.LowerMilli = lb.MilliValue()
		}
		if ub, ok := cr.UpperBound[corev1.ResourceCPU]; ok {
			rec.UpperMilli = ub.MilliValue()
		}
		return rec, true
	}
	return service.Recommendation{}, false
}
