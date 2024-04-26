package cpu

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	v1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMCPUReconciler struct {
	*vmattachee.AttacheeReconciler[*virtv2.VirtualMachineCPUModel, virtv2.VirtualMachineCPUModelStatus]
}

func NewVMCPUReconciler() *VMCPUReconciler {
	return &VMCPUReconciler{
		AttacheeReconciler: vmattachee.NewAttacheeReconciler[
			*virtv2.VirtualMachineCPUModel,
			virtv2.VirtualMachineCPUModelStatus,
		](),
	}
}

func (r *VMCPUReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineCPUModel{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Node{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromMapFunc(mgr.GetClient(), mgr.GetLogger())),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldNode, ok := e.ObjectOld.(*corev1.Node)
				if !ok {
					return false
				}

				newNode, ok := e.ObjectNew.(*corev1.Node)
				if !ok {
					return false
				}

				// TODO replace with maps.Equal() in go1.22.1.
				return !isEqualMaps(oldNode.GetLabels(), newNode.GetLabels())
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error setting watch on Pod: %w", err)
	}

	return r.AttacheeReconciler.SetupController(mgr, ctr, r)
}

func (r *VMCPUReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMCPUReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if r.AttacheeReconciler.Sync(ctx, state.AttacheeState, opts) {
		return nil
	}

	return nil
}

func (r *VMCPUReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMCPUReconcilerState, _ two_phase_reconciler.ReconcilerOptions) error {
	if state.isDeletion() {
		state.VMCPU.Changed().Status.Phase = virtv2.VMCPUPhaseTerminating

		return nil
	}

	modelNodes := r.getModelNodes(state.VMCPU.Current().Spec.Model, state.Nodes)
	features := r.getCommonNodeFeatures(state.Nodes)
	isReady := true

	switch state.VMCPU.Current().Spec.Type {
	case virtv2.Host:
	case virtv2.Model:
		state.VMCPU.Changed().Status.Nodes = &modelNodes
		isReady = len(modelNodes) > 0
	case virtv2.Features:
		state.VMCPU.Changed().Status.Nodes = &modelNodes
		state.VMCPU.Changed().Status.Features = &features
		isReady = containAll(features.Enabled, state.VMCPU.Current().Spec.Features...)
	default:
		return fmt.Errorf("%s: %w", state.VMCPU.Current().Spec.Type, common.ErrUnknownType)
	}

	if isReady {
		state.VMCPU.Changed().Status.Phase = virtv2.VMCPUPhaseReady

		return nil
	}

	state.VMCPU.Changed().Status.Phase = virtv2.VMCPUPhasePending

	return nil
}

func (r *VMCPUReconciler) FilterAttachedVM(vm *virtv2.VirtualMachine) bool {
	return vm.Spec.CPU.VirtualMachineCPUModel != ""
}

func (r *VMCPUReconciler) EnqueueFromAttachedVM(vm *virtv2.VirtualMachine) []reconcile.Request {
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name: vm.Spec.CPU.VirtualMachineCPUModel,
		},
	}}
}

func (r *VMCPUReconciler) getModelNodes(model string, nodes []corev1.Node) []string {
	var availableNodes []string

	for _, node := range nodes {
		for label, enabled := range node.Labels {
			if label != v1.CPUModelLabel+model || enabled != "true" {
				continue
			}

			availableNodes = append(availableNodes, node.Name)
		}
	}

	return availableNodes
}

func (r *VMCPUReconciler) getCommonNodeFeatures(nodes []corev1.Node) virtv2.VirtualMachineCPUModelStatusFeatures {
	enabledFeatures := make(map[string]int)
	disabledFeatures := make(map[string]int)

	for _, node := range nodes {
		for label, enabled := range node.Labels {
			if !strings.HasPrefix(label, v1.CPUFeatureLabel) {
				continue
			}

			if enabled == "true" {
				enabledFeatures[label] += 1
			} else {
				disabledFeatures[label] += 1
			}
		}
	}

	var features virtv2.VirtualMachineCPUModelStatusFeatures

	for feature, nodeCount := range enabledFeatures {
		if nodeCount != len(nodes) {
			continue
		}

		features.Enabled = append(
			features.Enabled,
			strings.TrimPrefix(feature, v1.CPUFeatureLabel),
		)
	}

	for feature, nodeCount := range disabledFeatures {
		if nodeCount != len(nodes) {
			continue
		}

		features.NotEnabledCommon = append(
			features.NotEnabledCommon,
			strings.TrimPrefix(feature, v1.CPUFeatureLabel),
		)
	}

	return features
}

func (r *VMCPUReconciler) enqueueRequestsFromMapFunc(c client.Client, logger logr.Logger) handler.MapFunc {
	return func(ctx context.Context, node client.Object) []reconcile.Request {
		var cpus virtv2.VirtualMachineCPUModelList
		err := c.List(ctx, &cpus)
		if err != nil {
			logger.Error(err, "Failed to list cpus")
			return nil
		}

		requests := make([]reconcile.Request, len(cpus.Items))
		for i, cpu := range cpus.Items {
			requests[i] = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: cpu.Name,
				},
			}
		}

		return requests
	}
}

func containAll(src []string, items ...string) bool {
	search := make(map[string]struct{}, len(items))
	for _, s := range src {
		search[s] = struct{}{}
	}

	for _, item := range items {
		_, ok := search[item]
		if !ok {
			return false
		}
	}

	return true
}

func isEqualMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for aKey, aValue := range a {
		bValue, ok := b[aKey]
		if !ok {
			return false
		}

		if aValue != bValue {
			return false
		}
	}

	return true
}
