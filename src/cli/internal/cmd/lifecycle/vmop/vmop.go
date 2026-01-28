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

package vmop

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type VirtualMachineOperation struct {
	client  kubeclient.Client
	options options
}

type options struct {
	force        bool
	waitComplete bool
	createOnly   bool
}

func New(client kubeclient.Client, opts ...func(*VirtualMachineOperation)) *VirtualMachineOperation {
	vmop := &VirtualMachineOperation{
		client:  client,
		options: options{},
	}

	for _, opt := range opts {
		opt(vmop)
	}

	return vmop
}

func WithForce(force bool) func(*VirtualMachineOperation) {
	return func(o *VirtualMachineOperation) {
		o.options.force = force
	}
}

func WithWaitComplete(waitComplete bool) func(*VirtualMachineOperation) {
	return func(o *VirtualMachineOperation) {
		o.options.waitComplete = waitComplete
	}
}

func WithCreateOnly(createOnly bool) func(*VirtualMachineOperation) {
	return func(o *VirtualMachineOperation) {
		o.options.createOnly = createOnly
	}
}

func (v VirtualMachineOperation) Stop(ctx context.Context, vmName, vmNamespace string) (msg string, err error) {
	vmop := v.newVMOP(vmName, vmNamespace, v1alpha2.VMOPTypeStop, false)
	return v.do(ctx, vmop, v.options.createOnly, v.options.waitComplete)
}

func (v VirtualMachineOperation) Start(ctx context.Context, vmName, vmNamespace string) (msg string, err error) {
	vmop := v.newVMOP(vmName, vmNamespace, v1alpha2.VMOPTypeStart, false)
	return v.do(ctx, vmop, v.options.createOnly, v.options.waitComplete)
}

func (v VirtualMachineOperation) Restart(ctx context.Context, vmName, vmNamespace string) (msg string, err error) {
	vmop := v.newVMOP(vmName, vmNamespace, v1alpha2.VMOPTypeRestart, v.options.force)
	return v.do(ctx, vmop, v.options.createOnly, v.options.waitComplete)
}

func (v VirtualMachineOperation) Evict(ctx context.Context, vmName, vmNamespace string) (msg string, err error) {
	vmop := v.newVMOP(vmName, vmNamespace, v1alpha2.VMOPTypeEvict, v.options.force)
	return v.do(ctx, vmop, v.options.createOnly, v.options.waitComplete)
}

func (v VirtualMachineOperation) Migrate(ctx context.Context, vmName, vmNamespace, targetNodeName string) (msg string, err error) {
	vmop := v.newVMOP(vmName, vmNamespace, v1alpha2.VMOPTypeMigrate, v.options.force)
	if targetNodeName != "" {
		vmop.Spec.Migrate = &v1alpha2.VirtualMachineOperationMigrateSpec{
			NodeSelector: map[string]string{corev1.LabelHostname: targetNodeName},
		}
	}
	return v.do(ctx, vmop, v.options.createOnly, v.options.waitComplete)
}

func (v VirtualMachineOperation) do(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, createOnly, waitCompleted bool) (msg string, err error) {
	if createOnly {
		vmop, err = v.create(ctx, vmop)
	} else {
		vmop, err = v.createAndWait(ctx, vmop, waitCompleted)
	}
	msg = v.generateMsg(vmop)
	return msg, err
}

func (v VirtualMachineOperation) generateMsg(vmop *v1alpha2.VirtualMachineOperation) string {
	if vmop == nil {
		return ""
	}
	key := types.NamespacedName{Namespace: vmop.GetNamespace(), Name: vmop.GetName()}
	vmKey := types.NamespacedName{Namespace: vmop.GetNamespace(), Name: vmop.Spec.VirtualMachine}
	phase := vmop.Status.Phase

	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("VirtualMachine %q ", vmKey.String()))

	if v.isPhaseOrFailed(vmop, v1alpha2.VMOPPhaseCompleted) {
		if !v.isCompleted(vmop) {
			sb.WriteString("was not ")
		}
		switch vmop.Spec.Type {
		case v1alpha2.VMOPTypeStart:
			sb.WriteString("started. ")
		case v1alpha2.VMOPTypeStop:
			sb.WriteString("stopped. ")
		case v1alpha2.VMOPTypeRestart:
			sb.WriteString("restarted. ")
		case v1alpha2.VMOPTypeEvict:
			sb.WriteString("evicted.")
		case v1alpha2.VMOPTypeMigrate:
			sb.WriteString("migrated.")
		}
	} else {
		switch vmop.Spec.Type {
		case v1alpha2.VMOPTypeStart:
			sb.WriteString("starting. ")
		case v1alpha2.VMOPTypeStop:
			sb.WriteString("stopping. ")
		case v1alpha2.VMOPTypeRestart:
			sb.WriteString("restarting. ")
		case v1alpha2.VMOPTypeEvict:
			sb.WriteString("evicting.")
		case v1alpha2.VMOPTypeMigrate:
			sb.WriteString("migrating.")
		}
	}

	sb.WriteString(fmt.Sprintf("VirtualMachineOperation %q ", key.String()))
	switch phase {
	case v1alpha2.VMOPPhasePending:
		sb.WriteString("pending.")
	case v1alpha2.VMOPPhaseInProgress:
		sb.WriteString("in progress.")
	case v1alpha2.VMOPPhaseCompleted:
		sb.WriteString("completed.")
	case v1alpha2.VMOPPhaseFailed:
		cond, _ := getCondition(vmopcondition.TypeCompleted.String(), vmop.Status.Conditions)
		sb.WriteString(fmt.Sprintf("failed. type=%q reason=%q, message=%q.", cond.Type, cond.Reason, cond.Message))
	case "":
		sb.WriteString("created.")
	default:
		sb.WriteString(fmt.Sprintf(" phase=%q.", phase))
	}
	sb.WriteString("\n")
	return sb.String()
}

func (v VirtualMachineOperation) createAndWait(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, waitCompleted bool) (*v1alpha2.VirtualMachineOperation, error) {
	vmop, err := v.create(ctx, vmop)
	if err != nil {
		return nil, err
	}
	if v.isPhaseOrFailed(vmop, v1alpha2.VMOPPhaseCompleted) {
		return vmop, nil
	}

	if waitCompleted {
		return v.waitUntil(ctx, vmop.GetName(), vmop.GetNamespace(), v1alpha2.VMOPPhaseCompleted)
	}

	return v.waitUntil(ctx, vmop.GetName(), vmop.GetNamespace(), v1alpha2.VMOPPhaseInProgress)
}

func (v VirtualMachineOperation) create(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*v1alpha2.VirtualMachineOperation, error) {
	return v.client.VirtualMachineOperations(vmop.GetNamespace()).Create(ctx, vmop, metav1.CreateOptions{})
}

func (v VirtualMachineOperation) waitUntil(ctx context.Context, name, namespace string, phase v1alpha2.VMOPPhase) (*v1alpha2.VirtualMachineOperation, error) {
	var vmop *v1alpha2.VirtualMachineOperation
	selector, err := fields.ParseSelector(fmt.Sprintf("metadata.name=%s", name))
	if err != nil {
		return nil, err
	}
	watcher, err := v.client.VirtualMachineOperations(namespace).Watch(ctx, metav1.ListOptions{FieldSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()
	for event := range watcher.ResultChan() {
		op, ok := event.Object.(*v1alpha2.VirtualMachineOperation)
		if !ok {
			continue
		}
		if v.isPhaseOrFailed(op, phase) {
			vmop = op
			break
		}
	}
	if !v.isPhaseOrFailed(vmop, phase) {
		return nil, context.DeadlineExceeded
	}
	return vmop, nil
}

func (v VirtualMachineOperation) isCompleted(vmop *v1alpha2.VirtualMachineOperation) bool {
	if vmop == nil {
		return false
	}
	return vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted
}

func (v VirtualMachineOperation) isPhaseOrFailed(vmop *v1alpha2.VirtualMachineOperation, phase v1alpha2.VMOPPhase) bool {
	if vmop == nil {
		return false
	}
	return vmop.Status.Phase == phase || vmop.Status.Phase == v1alpha2.VMOPPhaseFailed
}

func (v VirtualMachineOperation) newVMOP(vmName, vmNamespace string, t v1alpha2.VMOPType, force bool) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineOperationKind,
			APIVersion: v1alpha2.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vmName + "-",
			Namespace:    vmNamespace,
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           t,
			VirtualMachine: vmName,
			Force:          ptr.To(force),
		},
	}
}

func getCondition(condType string, conds []metav1.Condition) (metav1.Condition, bool) {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond, true
		}
	}

	return metav1.Condition{}, false
}
