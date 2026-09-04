/*
Copyright 2024 Flant JSC

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameEvictHandler = "EvictHandler"

func NewEvictHandler() *EvictHandler {
	return &EvictHandler{}
}

type EvictHandler struct{}

func (h *EvictHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachine().Changed()
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil || kvvmi.Status.Phase != virtv1.Running {
		conditions.RemoveCondition(vmcondition.TypeEvictionRequired, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	evictionStarted := kvvmi.Status.EvacuationNodeName != ""

	node, err := h.node(ctx, s, kvvmi.Status.NodeName)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !evictionStarted && !nodeIsUnderMaintenance(node) {
		conditions.RemoveCondition(vmcondition.TypeEvictionRequired, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	reason, message := h.outcome(changed.Status.Conditions, evictionStarted, restartApproved(node))

	conditions.SetCondition(
		conditions.NewConditionBuilder(vmcondition.TypeEvictionRequired).
			Generation(changed.GetGeneration()).
			Status(metav1.ConditionTrue).
			Reason(reason).
			Message(message),
		&changed.Status.Conditions,
	)
	return reconcile.Result{}, nil
}

func (h *EvictHandler) Name() string {
	return nameEvictHandler
}

// node returns the node the virtual machine runs on, or nil when there is none to look at.
func (h *EvictHandler) node(ctx context.Context, s state.VirtualMachineState, nodeName string) (*corev1.Node, error) {
	if nodeName == "" {
		return nil, nil
	}

	node := &corev1.Node{}
	err := s.Client().Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	return node, nil
}

// nodeIsUnderMaintenance reports whether the node is being taken out of service: it is closed for
// new workloads and marked by node-manager or by the shutdown inhibitor. A closed node alone is not
// enough — an administrator closes a node for other reasons too, and owners of virtual machines
// should not be warned then.
func nodeIsUnderMaintenance(node *corev1.Node) bool {
	if node == nil || !node.Spec.Unschedulable {
		return false
	}

	for _, annotation := range annotations.NodeMaintenanceMarkers() {
		if _, found := node.GetAnnotations()[annotation]; found {
			return true
		}
	}

	return false
}

// restartApproved reports whether the administrator allowed restarting machines that cannot leave
// this node. On an open node the permission means nothing: there is no maintenance to allow.
func restartApproved(node *corev1.Node) bool {
	if node == nil || !node.Spec.Unschedulable {
		return false
	}

	_, found := node.GetAnnotations()[annotations.AnnNodeVMRestartApproved]
	return found
}

// outcome tells what happens to the virtual machine and gives the owner the exact wording.
//
// Until an eviction arrives the machine keeps running and nothing is promised with a deadline:
// a closed node does not mean that a drain follows. Once the eviction has started, a machine that
// can be live migrated leaves the node without stopping the guest, and one that cannot is either
// restarted by the platform or blocks the node until a person moves it.
func (h *EvictHandler) outcome(currentConditions []metav1.Condition, evictionStarted, approved bool) (vmcondition.EvictionRequiredReason, string) {
	canLiveMigrate, targetMissing := migratability(currentConditions)

	// Until an eviction arrives nothing happens to the machine, whatever stands on the node: a
	// closed node is not a promise of a drain, and the approval alone asks for nothing. The owner
	// gets a warning about what a drain would mean, and that is all.
	if !evictionStarted {
		return vmcondition.ReasonNodeUnderMaintenance, maintenanceWarning(canLiveMigrate, targetMissing, approved)
	}

	// A machine that keeps its migratability leaves the node alive, so no restart is promised even
	// when one is allowed on this node. A target that is busy right now may be freed or reopened,
	// and until then the migration keeps being retried.
	if canLiveMigrate {
		if targetMissing {
			return vmcondition.ReasonEvictionRequired,
				"The node is being taken out of service. The virtual machine will be live migrated as soon as a node that can accept it becomes available; migration keeps being retried until then."
		}

		return vmcondition.ReasonEvictionRequired,
			"The node is being taken out of service. The virtual machine will be live migrated to another node without stopping the guest operating system."
	}

	// An administrator who allowed the restart is already waiting for the node, so no moment is
	// named: the machine is restarted as soon as it is clear it cannot leave.
	if approved {
		return vmcondition.ReasonRestartRequired, approvedRestartMessage()
	}

	return vmcondition.ReasonEvictionBlocked,
		"The node is being taken out of service, and the virtual machine cannot be live migrated. Restart the virtual machine to let the maintenance continue, or ask the cluster administrator to allow the restart."
}

// approvedRestartMessage tells the owner that the restart comes from a permission an administrator
// gave on the node, so no moment is named: the machine is restarted as soon as the platform sees it
// cannot leave.
func approvedRestartMessage() string {
	return "The node is being taken out of service, and the virtual machine cannot be live migrated. The cluster administrator has allowed restarting it, so the platform restarts it to release the node."
}

// migratability tells whether the machine can leave its node by live migration at all, and whether
// the only thing missing is a node to accept it. A machine that keeps its migratability is never
// restarted to release a node: it leaves alive, and a target that is busy right now may be freed or
// reopened at any moment. Only a machine that cannot be live migrated on its own configuration -
// a passed through device, the CPU model, local disks in CE, placement rules that match no node -
// is a candidate for a restart, because for such a machine nothing changes while it runs.
func migratability(currentConditions []metav1.Condition) (canLiveMigrate, targetMissing bool) {
	migratable, found := conditions.GetCondition(vmcondition.TypeMigratable, currentConditions)
	if !found {
		return false, false
	}

	return migratable.Status == metav1.ConditionTrue,
		migratable.Reason == vmcondition.ReasonWaitingForMigrationTarget.String()
}

// maintenanceWarning warns the owner before anything happens to the machine. The node is closed
// for new workloads and marked as going out of service, but no eviction has been requested yet:
// the administrator may reopen the node instead of draining it. Hence the conditional wording and
// no named moment — the warning tells what a drain would mean, not what is going to happen.
func maintenanceWarning(canLiveMigrate, targetMissing, approved bool) string {
	if canLiveMigrate {
		if targetMissing {
			return "The node is closed for new workloads. The virtual machine keeps running: if the node is drained, it will be live migrated as soon as a node that can accept it becomes available."
		}

		return "The node is closed for new workloads. The virtual machine keeps running: if the node is drained, it will be live migrated to another node."
	}

	if approved {
		return "The node is closed for new workloads, and the virtual machine cannot be live migrated. It keeps running: if the node is drained, the platform will restart it to release the node."
	}

	return "The node is closed for new workloads, and the virtual machine cannot be live migrated. It keeps running: if the node is drained, the node will not be released until you restart the machine."
}
