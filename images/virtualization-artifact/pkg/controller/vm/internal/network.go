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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		conditions.SetCondition(cb, &vm.Status.Conditions)
	}()

	if vm.Spec.Networks == nil {
		vm.Status.Networks = nil
		return reconcile.Result{}, nil
	}

	if len(vm.Spec.Networks) == 1 {
		vm.Status.Networks = []virtv2.NetworksStatus{
			{
				Type: virtv2.NetworksTypeMain,
			},
		}
		return reconcile.Result{}, nil
	}

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

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	macAddressesByInterfaceName := make(map[string]string)
	if kvvmi != nil {
		for _, i := range kvvmi.Status.Interfaces {
			macAddressesByInterfaceName[i.Name] = i.MAC
		}
	}

	vmmacs, err := s.VirtualMachineMACAddresses(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	vmmacNamesByAddress := make(map[string]string)
	for _, vmmac := range vmmacs {
		vmmacNamesByAddress[vmmac.Status.Address] = vmmac.Name
	}

	networksStatus := []virtv2.NetworksStatus{
		{
			Type: virtv2.NetworksTypeMain,
		},
	}

	for _, interfaceSpec := range network.CreateNetworkSpec(vm.Spec) {
		macAddress := macAddressesByInterfaceName[interfaceSpec.InterfaceName]
		networksStatus = append(networksStatus, virtv2.NetworksStatus{
			Type:                         interfaceSpec.Type,
			Name:                         interfaceSpec.Name,
			MAC:                          macAddress,
			VirtualMachineMACAddressName: vmmacNamesByAddress[macAddress],
		})
	}

	vm.Status.Networks = networksStatus
	return reconcile.Result{}, nil
}

func (h *NetworkInterfaceHandler) Name() string {
	return nameNetworkHandler
}

func extractNetworkStatusFromPods(pods *corev1.PodList) (string, error) {
	var errorMessages []string

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		networkStatusAnnotation, found := pod.Annotations[annotations.AnnNetworksStatus]
		if !found {
			errorMessages = append(errorMessages, fmt.Sprintf("Annotation %s is not found", annotations.AnnNetworksStatus))
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
