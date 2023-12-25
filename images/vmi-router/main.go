/*
Copyright 2023 Flant JSC

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

package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeschema "k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	virtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"vmi-router/controllers"
)

var (
	log                  = ctrl.Log.WithName("vmi-router")
	resourcesSchemeFuncs = []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		ciliumv2.AddToScheme,
		virtv1.AddToScheme,
	}
)

const kubevirtCoreGroupName = "x.virtualization.deckhouse.kubevirt.io"

func init() {
	overrideKubevirtCoreGroupName(kubevirtCoreGroupName)
}

func overrideKubevirtCoreGroupName(groupName string) {
	virtv1.GroupVersion.Group = groupName
	virtv1.SchemeGroupVersion.Group = groupName
	virtv1.StorageGroupVersion.Group = groupName
	for i := range virtv1.GroupVersions {
		virtv1.GroupVersions[i].Group = groupName
	}

	virtv1.VirtualMachineInstanceGroupVersionKind.Group = groupName
	virtv1.VirtualMachineInstanceReplicaSetGroupVersionKind.Group = groupName
	virtv1.VirtualMachineInstancePresetGroupVersionKind.Group = groupName
	virtv1.VirtualMachineGroupVersionKind.Group = groupName
	virtv1.VirtualMachineInstanceMigrationGroupVersionKind.Group = groupName
	virtv1.KubeVirtGroupVersionKind.Group = groupName

	virtv1.SchemeBuilder = runtime.NewSchemeBuilder(virtv1.AddKnownTypesGenerator([]runtimeschema.GroupVersion{virtv1.GroupVersion}))
	virtv1.AddToScheme = virtv1.SchemeBuilder.AddToScheme

	// Also override kubecli scheme related machinery.
	kubecli.SchemeBuilder = runtime.NewSchemeBuilder(virtv1.AddKnownTypesGenerator([]runtimeschema.GroupVersion{virtv1.GroupVersion}))
	kubecli.SchemeBuilder.AddToScheme(kubecli.Scheme)
	kubecli.SchemeBuilder.AddToScheme(clientgoscheme.Scheme)
}

type cidrFlag []string

func (f *cidrFlag) String() string { return "" }
func (f *cidrFlag) Set(s string) error {
	*f = append(*f, s)
	return nil
}

func main() {
	var cidrs cidrFlag
	var dryRun bool
	var metricsAddr string
	var probeAddr string
	flag.Var(&cidrs, "cidr", "CIDRs enabled to route (multiple flags allowed)")
	flag.BoolVar(&dryRun, "dry-run", false, "Don't perform any changes on the node.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":0", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":0", "The address the probe endpoint binds to.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var parsedCIDRs []*net.IPNet
	for _, cidr := range cidrs {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil || parsedCIDR == nil {
			fmt.Println(err, "failed to parse CIDR")
			os.Exit(1)
		}
		parsedCIDRs = append(parsedCIDRs, parsedCIDR)
	}

	log.Info(fmt.Sprintf("managed CIDRs: %+v", cidrs))

	// Setup scheme for all resources
	scheme := runtime.NewScheme()
	for _, f := range resourcesSchemeFuncs {
		err := f(scheme)
		if err != nil {
			log.Error(err, "Failed to add to scheme")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	clientSet, err := kubecli.GetKubevirtClientFromRESTConfig(mgr.GetConfig())
	if err != nil {
		log.Error(err, "unable to create clientset")
		os.Exit(1)
	}

	controller := controllers.VMIRouterController{
		RESTClient:        clientSet.RestClient(),
		Client:            mgr.GetClient(),
		CIDRs:             parsedCIDRs,
		RouteGet:          netlink.RouteGet,
		RouteDel:          netlink.RouteDel,
		RouteReplace:      netlink.RouteReplace,
		RuleAdd:           netlink.RuleAdd,
		RuleDel:           netlink.RuleDel,
		RuleListFiltered:  netlink.RuleListFiltered,
		RouteListFiltered: netlink.RouteListFiltered,
	}
	if dryRun {
		controller.RuleAdd = func(*netlink.Rule) error { return nil }
		controller.RuleDel = func(*netlink.Rule) error { return nil }
		controller.RouteDel = func(*netlink.Route) error { return nil }
		controller.RouteReplace = func(*netlink.Route) error { return nil }
	}

	if err := mgr.Add(controller); err != nil {
		log.Error(err, "unable to add vmi router controller to manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "problem running manager")
		os.Exit(1)
	}
}
