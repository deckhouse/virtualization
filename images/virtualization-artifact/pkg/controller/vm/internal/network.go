/*
Copyright 2025 Flant JSC

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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameNetworkHandler = "NetworkInterfaceHandler"

type NetworkInterfaceHandler struct {
	featureGate featuregate.FeatureGate
}

func NewNetworkInterfaceHandler(featureGate featuregate.FeatureGate) *NetworkInterfaceHandler {
	return &NetworkInterfaceHandler{
		featureGate: featureGate,
	}
}

func (h *NetworkInterfaceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeNetworkReady).
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown).
		Generation(vm.GetGeneration())

	defer func() {
		if cb.Condition().Status == metav1.ConditionUnknown {
			conditions.RemoveCondition(vmcondition.TypeNetworkReady, &vm.Status.Conditions)
		} else {
			conditions.SetCondition(cb, &vm.Status.Conditions)
		}
	}()

	if len(vm.Spec.Networks) > 1 {
		if !h.featureGate.Enabled(featuregates.SDN) {
			cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonSDNModuleDisable).Message("For additional network interfaces, please enable SDN module")
			return reconcile.Result{}, nil
		}

		pods, err := s.Pods(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}

		errMsg, err := extractNetworkStatusFromPods(pods)
		if err != nil {
			return reconcile.Result{}, err
		}

		if errMsg != "" {
			cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonNetworkNotReady).Message(errMsg)
		} else {
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonNetworkReady).Message("")
		}
	}

	return h.UpdateNetworkStatus(ctx, s, vm)
}

func (h *NetworkInterfaceHandler) Name() string {
	return nameNetworkHandler
}

func (h *NetworkInterfaceHandler) UpdateNetworkStatus(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	// check that vmmacName is not removed when deleting a network interface from the spec, as it is still in use
	if len(vm.Status.Networks) > len(vm.Spec.Networks) {
		if vm.Status.Phase != v1alpha2.MachinePending && vm.Status.Phase != v1alpha2.MachineStopped {
			return reconcile.Result{}, nil
		}
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	macAddressesByInterfaceName := make(map[string]string)
	if kvvm != nil && kvvm.Status.PrintableStatus != virtv1.VirtualMachineStatusUnschedulable {
		for _, i := range kvvm.Spec.Template.Spec.Domain.Devices.Interfaces {
			macAddressesByInterfaceName[i.Name] = i.MacAddress
		}
	}

	vmmacs, err := s.VirtualMachineMACAddresses(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	vmmacNamesByAddress := make(map[string]string)
	for _, vmmac := range vmmacs {
		if mac := vmmac.Status.Address; mac != "" {
			vmmacNamesByAddress[vmmac.Status.Address] = vmmac.Name
		}
	}

	networksStatus := []v1alpha2.NetworksStatus{
		{
			Type: v1alpha2.NetworksTypeMain,
			Name: "default",
		},
	}

	for _, interfaceSpec := range network.CreateNetworkSpec(vm, vmmacs) {
		networksStatus = append(networksStatus, v1alpha2.NetworksStatus{
			Type:                         interfaceSpec.Type,
			Name:                         interfaceSpec.Name,
			MAC:                          macAddressesByInterfaceName[interfaceSpec.InterfaceName],
			VirtualMachineMACAddressName: vmmacNamesByAddress[interfaceSpec.MAC],
		})
	}

	vm.Status.Networks = networksStatus
	return reconcile.Result{}, nil
}

func extractNetworkStatusFromPods(pods *corev1.PodList) (string, error) {
	var errorMessages []string

	if len(pods.Items) == 0 {
		return "Waiting for the pod to be created.", nil
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		networkStatusAnnotation, found := pod.Annotations[annotations.AnnNetworksStatus]
		if !found {
			if pod.Status.Phase == corev1.PodRunning {
				errorMessages = append(errorMessages, "Cannot determine the status of additional interfaces, waiting for a response from the SDN module")
			} else {
				errorMessages = append(errorMessages, "Waiting for virt-launcher pod to start")
			}
			continue
		}

		var interfacesStatus []network.InterfaceStatus
		if err := json.Unmarshal([]byte(networkStatusAnnotation), &interfacesStatus); err != nil {
			return "", err
		}

		podErrorMessages := collectInterfaceErrors(interfacesStatus)
		if len(podErrorMessages) > 0 {
			errorMessages = append(errorMessages, fmt.Sprintf("[%s]: %s", pod.Name, strings.Join(podErrorMessages, "; ")))
		}
	}

	return strings.Join(errorMessages, ". "), nil
}

func collectInterfaceErrors(interfacesStatus []network.InterfaceStatus) []string {
	var podErrorMessages []string
	for _, ifaceStatus := range interfacesStatus {
		ifaceErrMsgs := collectConditionErrors(ifaceStatus.Conditions)
		if len(ifaceErrMsgs) > 0 {
			podErrorMessages = append(podErrorMessages, fmt.Sprintf("(%s): %s", ifaceStatus.Name, strings.Join(ifaceErrMsgs, "; ")))
		}
	}
	return podErrorMessages
}

func collectConditionErrors(conditions []metav1.Condition) []string {
	var interfaceErrorMessages []string
	for _, condition := range conditions {
		if condition.Status != metav1.ConditionTrue {
			interfaceErrorMessages = append(interfaceErrorMessages, condition.Message)
		}
	}
	return interfaceErrorMessages
}
