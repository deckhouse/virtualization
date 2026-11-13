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

package checksdnrouteforger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const (
	moduleStateSecretSnapshot = "module-state-snapshot"
	moduleStateSecretName     = "module-state"

	// routeForgerSupportedCondition is set by the sdn module once every one of its
	// agents in the cluster runs the route controller that replaces vm-route-forge.
	routeForgerSupportedCondition = "PrimaryCNIRouteForgerSupported"

	routeForgerStateSecretKey  = "sdnRouteForgerDetection"
	routeForgerStateValuesPath = "virtualization.internal.moduleState." + routeForgerStateSecretKey

	routeForgerSchedule = "check-sdn-route-forger"

	// routeForgerSupportedMetric is 1 once vm-route-forge is no longer shipped; it
	// silences D8VirtualizationVMRouteForgeUnavailable.
	routeForgerSupportedMetric = "d8_virtualization_sdn_route_forger_supported"
)

var _ = registry.RegisterFunc(config, Reconcile)

// Runs before helm so the state is in values when vm-route-forge is rendered, and on
// a schedule: the sdn condition appears long after the module converged.
var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Schedule: []pkg.ScheduleConfig{
		{
			Name:    routeForgerSchedule,
			Crontab: "*/2 * * * *",
		},
	},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       moduleStateSecretSnapshot,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   `{data}`,
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{moduleStateSecretName},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
			ExecuteHookOnEvents:          ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

// Reconcile decides whether the module still ships vm-route-forge. The decision is
// one-way: a flapping answer would make the DaemonSet appear and disappear.
func Reconcile(ctx context.Context, input *pkg.HookInput) error {
	stateInSecret, err := readStateFromSecret(input)
	if err != nil {
		return err
	}

	// Values are empty after a deckhouse restart: copy the state so the Secret template
	// renders the same manifest.
	if stateInSecret != nil {
		input.Values.Set(routeForgerStateValuesPath, *stateInSecret)
		setRouteForgerSupportedMetric(input, true)

		return nil
	}

	// The Secret is gone but the state is in values: helm re-creates the Secret from them.
	if input.Values.Get(routeForgerStateValuesPath).Exists() {
		setRouteForgerSupportedMetric(input, true)
		return nil
	}

	sdnRoutesVMs, err := sdnRoutesVirtualMachines(ctx, input)
	if err != nil {
		// A failed read is not evidence of absence: keep shipping the DaemonSet
		// rather than drop the routing of a running fleet on a transient error.
		input.Logger.Error("Cannot check whether sdn routes virtual machines, keep shipping vm-route-forge", "error", err)
	}

	if !sdnRoutesVMs {
		setRouteForgerSupportedMetric(input, false)
		return nil
	}

	input.Logger.Info("The sdn module took over the routing of virtual machines, stop shipping vm-route-forge")
	input.Values.Set(routeForgerStateValuesPath, routeForgerState{DetectedAt: time.Now()})
	setRouteForgerSupportedMetric(input, true)

	return nil
}

func setRouteForgerSupportedMetric(input *pkg.HookInput, supported bool) {
	value := 0.0
	if supported {
		value = 1
	}

	input.MetricsCollector.Set(routeForgerSupportedMetric, value, nil)
}

type routeForgerState struct {
	DetectedAt time.Time `json:"detectedAt"`
}

func readStateFromSecret(input *pkg.HookInput) (*routeForgerState, error) {
	secrets := input.Snapshots.Get(moduleStateSecretSnapshot)
	if len(secrets) < 1 {
		return nil, nil
	}

	var secret corev1.Secret
	if err := secrets[0].UnmarshalTo(&secret); err != nil {
		return nil, err
	}

	stateJSON := secret.Data[routeForgerStateSecretKey]
	if len(stateJSON) == 0 {
		return nil, nil
	}

	var state routeForgerState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, fmt.Errorf("restore the route forger state from the Secret: %w", err)
	}

	return &state, nil
}

// sdnRoutesVirtualMachines is true once every sdn agent routes virtual machines, as the
// Main ClusterNetwork condition says, sdn knows the address of every vmip and can route
// every running VM on a vmip. Without the CRD or the network there is no such condition.
func sdnRoutesVirtualMachines(ctx context.Context, input *pkg.HookInput) (bool, error) {
	if input.DC == nil {
		return false, fmt.Errorf("dependency container is nil")
	}

	k8sClient, err := input.DC.GetK8sClient(schemeOption{})
	if err != nil {
		return false, fmt.Errorf("get kubernetes client: %w", err)
	}

	clusterNetwork := &sdnapi.ClusterNetwork{}

	err = k8sClient.Get(ctx, client.ObjectKey{Name: network.MainClusterNetworkName}, clusterNetwork)
	switch {
	case apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("get ClusterNetwork/%s: %w", network.MainClusterNetworkName, err)
	}

	if !meta.IsStatusConditionTrue(clusterNetwork.Status.Conditions, routeForgerSupportedCondition) {
		return false, nil
	}

	// Once vm-route-forge is gone, the sdn agents drop every route without an IPAddress:
	// wait until each vmip address has one.
	ctx = logger.ToContext(ctx, slog.New(input.Logger.Handler()))

	taken, err := network.VMIPAddressesTakenInSDNPool(ctx, k8sClient)
	if err != nil || !taken {
		return false, err
	}

	return vmsOnVMIPRoutedBySDN(ctx, k8sClient)
}

// vmsOnVMIPRoutedBySDN: an sdn agent routes an address only once its IPAddress is Bound or
// Attached and lists the pod using it. The mirror gets the pod a reconcile of the VM after
// it is allocated, when the pod gets the Main interface spec.
func vmsOnVMIPRoutedBySDN(ctx context.Context, c client.Client) (bool, error) {
	vms := &v1alpha2.VirtualMachineList{}
	if err := c.List(ctx, vms); err != nil {
		return false, fmt.Errorf("list VirtualMachine: %w", err)
	}

	vmips := &v1alpha2.VirtualMachineIPAddressList{}
	if err := c.List(ctx, vmips); err != nil {
		return false, fmt.Errorf("list VirtualMachineIPAddress: %w", err)
	}

	ipas := &sdnapi.IPAddressList{}
	if err := c.List(ctx, ipas); err != nil {
		return false, fmt.Errorf("list SDN IPAddress: %w", err)
	}

	vmipByName := make(map[types.NamespacedName]*v1alpha2.VirtualMachineIPAddress, len(vmips.Items))
	for i := range vmips.Items {
		vmipByName[types.NamespacedName{Namespace: vmips.Items[i].Namespace, Name: vmips.Items[i].Name}] = &vmips.Items[i]
	}

	for i := range vms.Items {
		vm := &vms.Items[i]

		pod := activePodName(vm)
		if pod == "" || vm.Status.VirtualMachineIPAddress == "" {
			continue
		}

		vmip := vmipByName[types.NamespacedName{Namespace: vm.Namespace, Name: vm.Status.VirtualMachineIPAddress}]
		if vmip == nil || !mirrorRoutesPod(ipas.Items, vmip, vm.Namespace+"/"+pod) {
			logger.FromContext(ctx).Debug("sdn cannot route a running VM on a vmip yet", "vm", vm.Namespace+"/"+vm.Name, "pod", pod)
			return false, nil
		}
	}

	return true, nil
}

func activePodName(vm *v1alpha2.VirtualMachine) string {
	for _, pod := range vm.Status.VirtualMachinePods {
		if pod.Active {
			return pod.Name
		}
	}

	return ""
}

func mirrorRoutesPod(ipas []sdnapi.IPAddress, vmip *v1alpha2.VirtualMachineIPAddress, pod string) bool {
	for i := range ipas {
		ipa := &ipas[i]
		if !network.IsMirrorOf(ipa, vmip) || ipa.Status.Address != vmip.Status.Address {
			continue
		}

		if ipa.Status.Phase == sdnapi.PhaseBound || ipa.Status.Phase == sdnapi.PhaseAttached {
			return slices.Contains(ipa.Status.UsedByPods, pod)
		}
	}

	return false
}

type schemeOption struct{}

func (schemeOption) Apply(optsApplier pkg.KubernetesOptionApplier) {
	optsApplier.WithSchemeBuilder(sdnapi.SchemeBuilder)
	optsApplier.WithSchemeBuilder(v1alpha2.SchemeBuilder)
}
