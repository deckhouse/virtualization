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

package vm

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

// VMContainerNameSuffix - a name suffix for container with virt-launcher, libvirt and qemu processes.
// Container name is "d8v-compute", but previous versions may have "compute" container.
const VMContainerNameSuffix = "compute"

// CalculateCoresAndSockets calculates the number of sockets and cores per socket needed to achieve
// the desired total number of CPU cores.
// The function tries to minimize the number of sockets while ensuring the desired core count.
//
// https://bugzilla.redhat.com/show_bug.cgi?id=1653453
// The number of cores per socket and the growth of the number of sockets is chosen in such a way as
// to have less impact on the performance of the virtual machine, as well as on compatibility with operating systems
func CalculateCoresAndSockets(desiredCores int) (sockets, coresPerSocket int) {
	if desiredCores < 1 {
		return 1, 1
	}

	if desiredCores <= 16 {
		return 1, desiredCores
	}

	switch {
	case desiredCores <= 32:
		sockets = 2
	case desiredCores <= 64:
		sockets = 4
	default:
		sockets = 8
	}

	coresPerSocket = desiredCores / sockets
	if desiredCores%sockets != 0 {
		coresPerSocket++
	}

	return sockets, coresPerSocket
}

func ApprovalMode(vm *v1alpha2.VirtualMachine) v1alpha2.RestartApprovalMode {
	if vm.Spec.Disruptions == nil {
		return v1alpha2.Manual
	}
	return vm.Spec.Disruptions.RestartApprovalMode
}

func RestartRequired(vm *v1alpha2.VirtualMachine) bool {
	if vm == nil {
		return false
	}

	cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}

func IsComputeContainer(name string) bool {
	return strings.HasSuffix(name, VMContainerNameSuffix)
}

func IsVMActive(ctx context.Context, cli client.Client, vm v1alpha2.VirtualMachine) (bool, error) {
	kvvm, err := object.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, cli, &virtv1.VirtualMachine{})
	if err != nil {
		return false, fmt.Errorf("error getting kvvms: %w", err)
	}
	if kvvm != nil && kvvm.Status.StateChangeRequests != nil {
		return true, nil
	}

	podList := corev1.PodList{}
	err = cli.List(ctx, &podList, &client.ListOptions{
		Namespace:     vm.GetNamespace(),
		LabelSelector: labels.SelectorFromSet(map[string]string{virtv1.VirtualMachineNameLabel: vm.GetName()}),
	})
	if err != nil {
		return false, fmt.Errorf("unable to list virt-launcher Pod for VM %q: %w", vm.GetName(), err)
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return true, nil
		}
	}

	return false, nil
}

func GetActivePodName(vm *v1alpha2.VirtualMachine) (string, bool) {
	for _, pod := range vm.Status.VirtualMachinePods {
		if pod.Active {
			return pod.Name, true
		}
	}

	return "", false
}
