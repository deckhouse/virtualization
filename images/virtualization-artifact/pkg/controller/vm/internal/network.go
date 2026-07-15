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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	if !hasOnlyDefaultNetwork(vm) {
		if err := h.evaluateAdditionalNetworks(ctx, s, vm, cb); err != nil {
			return reconcile.Result{}, err
		}
	}

	return h.UpdateNetworkStatus(ctx, s, vm)
}

// evaluateAdditionalNetworks sets cb based on whether additional networks are
// usable: requires SDN feature gate, then that referenced Networks/ClusterNetworks
// are Ready, and finally that SDN reports the per-pod interfaces healthy.
func (h *NetworkInterfaceHandler) evaluateAdditionalNetworks(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine, cb *conditions.ConditionBuilder) error {
	if !h.featureGate.Enabled(featuregates.SDN) {
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonSDNModuleDisabled).Message("For additional network interfaces, please enable SDN module")
		return nil
	}

	var pending, desired []string
	for _, netSpec := range vm.Spec.Networks {
		ready, err := network.IsNetworkSpecReady(ctx, s.Client(), vm.Namespace, netSpec)
		if err != nil {
			return err
		}
		if !ready {
			pending = append(pending, network.SpecKey(netSpec))
			continue
		}
		if netSpec.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		// Only expect SDN status for interfaces that will actually be included
		// in networks-spec. Skipped interfaces (no pool + ipAddressName, or
		// IPAddress not yet allocated/exists) are not provisioned by SDN, so
		// waiting for their status would produce a misleading message.
		willProvision, err := network.WillProvisionInterface(ctx, s.Client(), vm.Namespace, vm, netSpec)
		if err != nil {
			return err
		}
		if willProvision {
			desired = append(desired, netSpec.Name)
		}
	}
	if len(pending) > 0 {
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonNetworkNotReady).
			Message(fmt.Sprintf("Waiting for the following networks to become Ready: %s", strings.Join(pending, ", ")))
		return nil
	}

	pods, err := s.Pods(ctx)
	if err != nil {
		return err
	}
	errMsg, err := extractNetworkStatusFromPods(pods, desired)
	if err != nil {
		return err
	}

	// Aggregate IPAM configuration errors for additional networks.
	// A network with ipAddressName set requires a pool (IPAM) on the referenced
	// Network/ClusterNetwork; otherwise the IPAddress cannot be applied.
	ipamErrors := collectIPAMErrors(ctx, s.Client(), vm.Namespace, vm, vm.Spec.Networks)
	if len(ipamErrors) > 0 {
		if errMsg != "" {
			errMsg += ". " + strings.Join(ipamErrors, "; ")
		} else {
			errMsg = strings.Join(ipamErrors, "; ")
		}
	}
	if errMsg != "" {
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonNetworkNotReady).Message(errMsg)
		return nil
	}
	cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonNetworkReady).Message("")
	return nil
}

func hasOnlyDefaultNetwork(vm *v1alpha2.VirtualMachine) bool {
	nets := vm.Spec.Networks
	return len(nets) == 0 || (len(nets) == 1 && nets[0].Type == v1alpha2.NetworksTypeMain)
}

// collectIPAMErrors returns per-network IPAM configuration errors to aggregate
// into the NetworkReady condition message.
//
// Errors reported (the VM is not blocked from running by these; problematic
// interfaces are skipped by EnrichWithIPAM, and the errors are surfaced here):
//   - static mode: ipAddressName set but the network has no pool (IPAM);
//   - static mode: referenced IPAddress does not exist or is not bound to the network;
//   - auto mode: the IPAddress created by the controller is not yet allocated
//     (Pending / NoFreeIPAddress — pool exhausted).
func collectIPAMErrors(ctx context.Context, c client.Client, namespace string, vm *v1alpha2.VirtualMachine, networks []v1alpha2.NetworksSpec) []string {
	var errs []string
	for _, netSpec := range networks {
		if netSpec.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		hasPool, err := network.HasIPAM(ctx, c, namespace, netSpec)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: failed to check IPAM configuration: %v", network.SpecKey(netSpec), err))
			continue
		}
		if !hasPool {
			// No pool: if user set ipAddressName, it's a config error; otherwise L2-only is fine.
			if netSpec.IPAddressName != "" {
				errs = append(errs, fmt.Sprintf(
					"%s: ipAddressName %q is set but the network has no IPAM pool configured; the IPAddress cannot be applied",
					network.SpecKey(netSpec), netSpec.IPAddressName))
			}
			continue
		}

		// Pool exists: validate the IPAddress (static or auto).
		if netSpec.IPAddressName != "" {
			// Static mode: check the user-provided IPAddress exists, matches the network, and is allocated.
			exists, err := network.SDNIPAddressExists(ctx, c, namespace, netSpec.IPAddressName, netSpec.Type, netSpec.Name)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to check IPAddress %q: %v", network.SpecKey(netSpec), netSpec.IPAddressName, err))
				continue
			}
			if !exists {
				errs = append(errs, fmt.Sprintf(
					"%s: ipAddressName %q does not exist or is not bound to this network; the interface is skipped",
					network.SpecKey(netSpec), netSpec.IPAddressName))
				continue
			}
			status, err := network.GetSDNIPAddressStatus(ctx, c, namespace, netSpec.IPAddressName)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to get IPAddress %q status: %v", network.SpecKey(netSpec), netSpec.IPAddressName, err))
				continue
			}
			if status == nil || !status.Allocated {
				reason := ipStatusReason(status)
				phase := ipStatusPhase(status)
				errs = append(errs, fmt.Sprintf(
					"%s: ipAddressName %q is in phase %s (%s); the interface is skipped",
					network.SpecKey(netSpec), netSpec.IPAddressName, phase, reason))
				continue
			}
			conflictVM, err := network.IsIPAddressNameUsedByAnotherVM(ctx, c, vm, netSpec.IPAddressName, netSpec)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to check IPAddress %q conflict: %v", network.SpecKey(netSpec), netSpec.IPAddressName, err))
				continue
			}
			if conflictVM != "" {
				errs = append(errs, fmt.Sprintf(
					"%s: ipAddressName %q is already used by VM %q; the interface is skipped",
					network.SpecKey(netSpec), netSpec.IPAddressName, conflictVM))
			}
			continue
		}

		// Auto mode: check the controller-created IPAddress is allocated.
		name, err := network.FindSDNIPAddress(ctx, c, vm, netSpec)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: failed to find auto IPAddress: %v", network.SpecKey(netSpec), err))
			continue
		}
		if name == "" {
			errs = append(errs, fmt.Sprintf("%s: auto IPAddress is not yet created; waiting for the controller", network.SpecKey(netSpec)))
			continue
		}
		status, err := network.GetSDNIPAddressStatus(ctx, c, namespace, name)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: failed to get IPAddress %q status: %v", network.SpecKey(netSpec), name, err))
			continue
		}
		if status == nil || !status.Allocated {
			reason := ipStatusReason(status)
			phase := ipStatusPhase(status)
			errs = append(errs, fmt.Sprintf(
				"%s: auto IPAddress %q is in phase %s (%s); the interface is skipped",
				network.SpecKey(netSpec), name, phase, reason))
		}
	}
	return errs
}

// ipStatusReason returns a human-readable reason for why an IPAddress is not allocated.
func ipStatusReason(status *network.SDNIPAddressStatus) string {
	if status == nil {
		return "IPAddress not found"
	}
	if status.NoFreeAddress {
		return "pool exhausted (NoFreeIPAddress)"
	}
	return "address not yet allocated"
}

// ipStatusPhase returns the phase of an IPAddress, or "unknown" if nil.
func ipStatusPhase(status *network.SDNIPAddressStatus) string {
	if status == nil || status.Phase == "" {
		return "unknown"
	}
	return status.Phase
}

func (h *NetworkInterfaceHandler) Name() string {
	return nameNetworkHandler
}

func (h *NetworkInterfaceHandler) UpdateNetworkStatus(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if hasOnlyDefaultNetwork(vm) {
		vm.Status.Networks = []v1alpha2.NetworksStatus{
			{
				ID:   network.ReservedMainID,
				Type: v1alpha2.NetworksTypeMain,
			},
		}
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	kvvmi, err := s.KVVMI(ctx)
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

	// Collect IP addresses allocated by SDN for additional interfaces from the
	// pod's network.deckhouse.io/networks-status annotation (ipAddressConfigs).
	ipAddressesByName := make(map[string]string)
	if pods, err := s.Pods(ctx); err == nil {
		ipAddressesByName = extractIPAddressesFromPods(pods)
	}

	var networksStatus []v1alpha2.NetworksStatus
	for _, interfaceSpec := range network.CreateNetworkSpec(vm, vmmacs) {
		if interfaceSpec.Type == v1alpha2.NetworksTypeMain {
			networksStatus = append(networksStatus, v1alpha2.NetworksStatus{
				ID:   interfaceSpec.ID,
				Type: v1alpha2.NetworksTypeMain,
			})
			continue
		}

		networksStatus = append(networksStatus, v1alpha2.NetworksStatus{
			ID:                           interfaceSpec.ID,
			Type:                         interfaceSpec.Type,
			Name:                         interfaceSpec.Name,
			MAC:                          macAddressesByInterfaceName[interfaceSpec.InterfaceName],
			VirtualMachineMACAddressName: vmmacNamesByAddress[interfaceSpec.MAC],
			IPAddress:                    ipAddressesByName[interfaceSpec.Name],
		})
	}

	// Network hot-unplug: retain a status entry the user removed from spec until
	// KubeVirt fully detaches and drops the iface from KVVMI. The next reconcile
	// then drops the entry, vmmac becomes unattached, deletion handler releases the MAC.
	for _, prev := range vm.Status.Networks {
		if prev.Type == v1alpha2.NetworksTypeMain || prev.MAC == "" {
			continue
		}
		if slices.ContainsFunc(networksStatus, func(networkStatus v1alpha2.NetworksStatus) bool {
			return networkStatus.Type == prev.Type && networkStatus.Name == prev.Name
		}) {
			continue
		}
		if kvvmi == nil || !slices.ContainsFunc(kvvmi.Spec.Domain.Devices.Interfaces, func(i virtv1.Interface) bool {
			return strings.EqualFold(i.MacAddress, prev.MAC)
		}) {
			continue
		}
		networksStatus = append(networksStatus, prev)
	}

	vm.Status.Networks = networksStatus
	return reconcile.Result{}, nil
}

func extractNetworkStatusFromPods(pods *corev1.PodList, desired []string) (string, error) {
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

		if len(desired) > 0 {
			present := make(map[string]struct{}, len(interfacesStatus))
			for _, ifs := range interfacesStatus {
				present[ifs.Name] = struct{}{}
			}
			for _, name := range desired {
				if _, ok := present[name]; !ok {
					podErrorMessages = append(podErrorMessages, fmt.Sprintf("(%s): waiting for SDN to report status", name))
				}
			}
		}

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

// extractIPAddressesFromPods returns a map of additional network name to the IP
// address allocated by SDN, parsed from the network.deckhouse.io/networks-status
// annotation (ipAddressConfigs[].address) of the virt-launcher pods.
// If multiple pods report the same network, the first non-empty address wins.
// Errors parsing the annotation are ignored: the status is best-effort and the
// readiness path (extractNetworkStatusFromPods) already surfaces SDN errors.
func extractIPAddressesFromPods(pods *corev1.PodList) map[string]string {
	result := make(map[string]string)
	if pods == nil {
		return result
	}
	for _, pod := range pods.Items {
		annotation, found := pod.Annotations[annotations.AnnNetworksStatus]
		if !found {
			continue
		}
		var interfacesStatus []network.InterfaceStatus
		if err := json.Unmarshal([]byte(annotation), &interfacesStatus); err != nil {
			continue
		}
		for _, ifs := range interfacesStatus {
			if ifs.Name == "" {
				continue
			}
			for _, cfg := range ifs.IPAddressConfigs {
				if cfg.Address != "" {
					if _, exists := result[ifs.Name]; !exists {
						result[ifs.Name] = cfg.Address
					}
					break
				}
			}
		}
	}
	return result
}
