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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const nameAutoCoreFractionHandler = "AutoCoreFractionHandler"

// AutoCoreFractionHandler drives the coreFraction of VMs opted into vertical
// autoscaling (spec.cpu.coreFraction == "Auto"). It owns one VPA per such VM, named
// after the VM UID (updateMode Off, so the recommender never evicts the pod), seeds
// status.recommendedResources.cpu.coreFraction on first sight, and moves it to the value derived
// from the VPA recommendation. SyncKvvm resolves "Auto" to that number and applies it in
// place. There is no cross-VM orchestration: a VM that leaves the recommended range
// is moved, up or down. Such a VM also carries the CoreFractionAutoscaling condition,
// which reports whether the platform still drives its core fraction.
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

	vm := s.VirtualMachine().Changed()

	if vm.GetDeletionTimestamp() != nil {
		if !autoscaled(vm) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, h.deleteVPA(ctx, vm)
	}

	// The condition only makes sense for a VM that asked for an automatic core fraction:
	// it stays Unknown for every other VM and is dropped from the status by the deferred
	// call below.
	cb := conditions.NewConditionBuilder(vmcondition.TypeCoreFractionAutoscaling).
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown).
		Generation(vm.GetGeneration())

	defer func() {
		if cb.Condition().Status == metav1.ConditionUnknown {
			conditions.RemoveCondition(vmcondition.TypeCoreFractionAutoscaling, &vm.Status.Conditions)
		} else {
			conditions.SetCondition(cb, &vm.Status.Conditions)
		}
	}()

	// Autoscaling is opt-in per VM. When off, drop the VPA and retract the driven value;
	// a VM that was never autoscaled carries none, so skip to avoid a delete per reconcile.
	// Such a VM carries no condition either — the deferred call above removes it.
	if vm.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		if sizingpolicy.RecommendedCoreFraction(vm) == "" {
			return reconcile.Result{}, nil
		}
		if err := h.deleteVPA(ctx, vm); err != nil {
			return reconcile.Result{}, err
		}
		sizingpolicy.SetRecommendedCoreFraction(vm, "")
		return reconcile.Result{}, nil
	}

	// From here on the VM asked for an automatic core fraction. Everything that can take
	// the automation away from it — a disabled feature, a narrowed sizing policy — leaves
	// the VM on the core fraction it has (the resolver keeps reading it from the status)
	// and says why in the condition. Its VPA goes away: nothing would act on it.
	if !h.featureGate.Enabled(featuregates.VerticalVirtualMachineAutoscaler) {
		if err := h.deleteVPA(ctx, vm); err != nil {
			return reconcile.Result{}, err
		}
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonCoreFractionAutoscalingDisabled).
			Message(unavailableMessage("vertical autoscaling of virtual machines is disabled",
				"To manage it yourself, set an explicit value in the specification."))
		return reconcile.Result{}, nil
	}

	if !h.featureGate.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize) {
		if err := h.deleteVPA(ctx, vm); err != nil {
			return reconcile.Result{}, err
		}
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonInPlaceResizeDisabled).
			Message(unavailableMessage("in-place resizing of virtual machine resources is disabled",
				"To manage it yourself, set an explicit value in the specification."))
		return reconcile.Result{}, nil
	}

	class, err := s.Class(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if class == nil {
		// The class may show up later; until then there is no policy to pick steps from.
		cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonWaitingForRecommendation).
			Message(waitingForRecommendationMessage)
		return reconcile.Result{}, nil
	}

	// The webhook rejects "Auto" against a policy with fewer than two steps, but the class
	// lives its own life: an administrator may narrow the policy under a running VM. We do
	// not forbid that — VMs of the class that use an explicit fraction are unaffected — so
	// the VM sorts it out for itself: it keeps its current core fraction and reports why.
	policy := sizingpolicy.MatchSizingPolicy(class, vm.Spec.CPU.Cores)
	if policy != nil && !sizingpolicy.CanAutoscaleCoreFraction(policy) {
		if err := h.deleteVPA(ctx, vm); err != nil {
			return reconcile.Result{}, err
		}
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonSizingPolicyHasNoSteps).
			Message(noStepsMessage(class, policy))
		return reconcile.Result{}, nil
	}

	if err := h.ensureVPA(ctx, vm); err != nil {
		return reconcile.Result{}, err
	}

	// Until the recommender has something to say, the VM runs on the seed: a low Burstable
	// value it can climb from.
	cb.Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonWaitingForRecommendation).
		Message(waitingForRecommendationMessage)

	current := sizingpolicy.RecommendedCoreFraction(vm)
	if current == "" {
		sizingpolicy.SetRecommendedCoreFraction(vm, fmt.Sprintf("%d%%", sizingpolicy.SeedAutoCoreFraction(class, vm.Spec.CPU.Cores)))
		return reconcile.Result{}, nil
	}

	vpaObj := &vpav1.VerticalPodAutoscaler{}
	if err := h.client.Get(ctx, vpaKey(vm), vpaObj); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("get VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), commonvm.VerticalPodAutoscalerName(vm), err)
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if override, ok, err := recommendationOverride(kvvm); err != nil {
		logger.FromContext(ctx).Warn("Ignoring malformed recommendation override annotation", logger.SlogErr(err))
	} else if ok {
		vpaObj.Status.Recommendation = override
	}

	rec, ok := computeCPURecommendation(vpaObj)
	if !ok {
		return reconcile.Result{}, nil
	}

	// There is a recommendation to act on: autoscaling is fully up.
	cb.Status(metav1.ConditionTrue).
		Reason(vmcondition.ReasonCoreFractionAutoscalingEnabled).
		Message("The CPU core fraction is selected automatically.")

	decision, err := h.coreFractioner.Calculate(vm, class, rec)
	if err != nil {
		return reconcile.Result{}, err
	}
	if decision.Direction == service.DirectionNone {
		return reconcile.Result{}, nil
	}

	desired := fmt.Sprintf("%d%%", decision.DesiredCoreFraction)
	if desired == current {
		return reconcile.Result{}, nil
	}

	logger.FromContext(ctx).Info("Updating desired coreFraction from VPA recommendation",
		slog.String("direction", decision.Direction.String()),
		slog.String("from", current),
		slog.String("to", desired),
	)
	h.recorder.Eventf(vm, corev1.EventTypeNormal, v1alpha2.ReasonCoreFractionScaling,
		"Scaling CPU core fraction %s from %s to %s.", decision.Direction, current, desired)
	sizingpolicy.SetRecommendedCoreFraction(vm, desired)

	return reconcile.Result{}, nil
}

func (h *AutoCoreFractionHandler) Name() string {
	return nameAutoCoreFractionHandler
}

func (h *AutoCoreFractionHandler) ensureVPA(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	name := commonvm.VerticalPodAutoscalerName(vm)

	existing := &vpav1.VerticalPodAutoscaler{}
	err := h.client.Get(ctx, vpaKey(vm), existing)
	if err == nil {
		// The VPA never changes after creation: its spec is derived from the VM name alone.
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), name, err)
	}

	desired := newVPAForVirtualMachine(vm)
	if err := controllerutil.SetControllerReference(vm, desired, h.scheme); err != nil {
		return fmt.Errorf("set owner reference on VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), name, err)
	}

	if err := h.client.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), name, err)
	}
	return nil
}

// deleteVPA removes the VPA owned by the VM. The owner reference alone would eventually
// have the garbage collector do it, but deleting here frees the recommender from a VM
// that no longer autoscales without waiting for the VM to actually go away. The name is
// derived from the VM UID, so this can never hit a VPA the user owns.
//
// A missing CRD is not an error: this also runs with the feature switched off, and the
// gate goes off exactly when the VPA CRD is gone — then there is nothing left to delete.
func (h *AutoCoreFractionHandler) deleteVPA(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	name := commonvm.VerticalPodAutoscalerName(vm)

	obj := &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vm.GetNamespace()},
	}
	if err := h.client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return fmt.Errorf("delete VerticalPodAutoscaler %s/%s: %w", vm.GetNamespace(), name, err)
	}
	return nil
}

// autoscaled reports whether the VM may own a VPA: either it is opted into autoscaling
// right now, or it still carries the value the autoscaler drove before it opted out.
func autoscaled(vm *v1alpha2.VirtualMachine) bool {
	return vm.Spec.CPU.CoreFraction == v1alpha2.CoreFractionAuto || sizingpolicy.RecommendedCoreFraction(vm) != ""
}

// waitingForRecommendationMessage covers every reason the autoscaler has nothing to act on
// yet — the VPA has just been created, its recommender has not filled the status, or the
// VirtualMachineClass is not there yet. For the user they are one state: autoscaling is on,
// the initial value holds until enough usage data is collected.
const waitingForRecommendationMessage = "The CPU core fraction is selected automatically. " +
	"The initial value is used until enough CPU usage data is collected."

// unavailableMessage explains that the VM asked for an automatic core fraction the platform
// cannot provide, and that it is not left without one: the value it has stays in effect.
func unavailableMessage(cause, advice string) string {
	return fmt.Sprintf("The CPU core fraction cannot be selected automatically: %s. "+
		"The virtual machine keeps its current core fraction. %s", cause, advice)
}

// noStepsMessage explains that the sizing policy of the class leaves the autoscaler nothing
// to choose between, and points at the administrator: the policy is theirs to widen.
func noStepsMessage(class *v1alpha2.VirtualMachineClass, policy *v1alpha2.SizingPolicy) string {
	allows := "allows no core fraction below 100%"
	if len(sizingpolicy.AutoCoreFractionSteps(policy)) > 0 {
		allows = "allows a single core fraction below 100%"
	}
	return unavailableMessage(
		fmt.Sprintf("the sizing policy of the VirtualMachineClass %q %s", class.GetName(), allows),
		"Ask the administrator to allow more values, or set an explicit one in the specification.")
}

func vpaKey(vm *v1alpha2.VirtualMachine) types.NamespacedName {
	return types.NamespacedName{Name: commonvm.VerticalPodAutoscalerName(vm), Namespace: vm.GetNamespace()}
}

// newVPAForVirtualMachine builds the VPA for a VM in updateMode Off. Its name comes from
// the VM UID so that it cannot clash with a VPA the user created for a same-named
// workload; the VM is set as the controller owner, which both ties the object to this VM
// and lets the garbage collector clean it up. targetRef points at the subresources API
// group, not the core CRD group: the /scale subresource is served there by the aggregated
// apiserver, and the recommender only treats a targetRef as scalable when its group/kind
// exposes /scale in discovery.
func newVPAForVirtualMachine(vm *v1alpha2.VirtualMachine) *vpav1.VerticalPodAutoscaler {
	return &vpav1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      commonvm.VerticalPodAutoscalerName(vm),
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

// recommendationOverride returns the recommendation pinned on the internal VirtualMachine
// via [annotations.AnnRecommendationOverride], if the annotation is present and
// well-formed. A malformed value returns an error so the caller can log and fall back to
// the recommender's own status rather than break the reconcile.
func recommendationOverride(kvvm *virtv1.VirtualMachine) (*vpav1.RecommendedPodResources, bool, error) {
	if kvvm == nil {
		return nil, false, nil
	}
	raw, ok := kvvm.GetAnnotations()[annotations.AnnRecommendationOverride]
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
