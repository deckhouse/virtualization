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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	log                  = logf.Log.WithName("cmd")
	resourcesSchemeFuncs = []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		extv1.AddToScheme,
		virtv1.AddToScheme,
		v1alpha2.AddToScheme,
	}
)

const (
	podNamespaceVar  = "POD_NAMESPACE"
	defaultVerbosity = "1"
)

func setupLogger() {
	verbose := defaultVerbosity
	if verboseEnvVarVal := os.Getenv("VERBOSITY"); verboseEnvVarVal != "" {
		verbose = verboseEnvVarVal
	}
	// visit actual flags passed in and if passed check -v and set verbose
	if fv := flag.Lookup("v"); fv != nil {
		verbose = fv.Value.String()
	}
	if verbose == defaultVerbosity {
		log.V(1).Info(fmt.Sprintf("Note: increase the -v level in the controller deployment for more detailed logging, eg. -v=%d or -v=%d\n", 2, 3))
	}
	verbosityLevel, err := strconv.Atoi(verbose)
	debug := false
	if err == nil && verbosityLevel > 1 {
		debug = true
	}

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.New(zap.Level(zapcore.Level(-1*verbosityLevel)), zap.UseDevMode(debug)))
}

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func main() {
	flag.Parse()

	setupLogger()
	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	leaderElectionNS := os.Getenv(podNamespaceVar)
	if leaderElectionNS == "" {
		leaderElectionNS = "default"
	}

	// Setup scheme for all resources
	scheme := apiruntime.NewScheme()
	for _, f := range resourcesSchemeFuncs {
		err = f(scheme)
		if err != nil {
			log.Error(err, "Failed to add to scheme")
			os.Exit(1)
		}
	}

	managerOpts := manager.Options{
		// This controller watches resources in all namespaces.
		LeaderElection:             false,
		LeaderElectionNamespace:    leaderElectionNS,
		LeaderElectionID:           "test-controller-leader-election-helper",
		LeaderElectionResourceLock: "leases",
		Scheme:                     scheme,
	}

	// Create a new Manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, managerOpts)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Bootstrapping the Manager.")

	// Setup context to gracefully handle termination.
	ctx := signals.SetupSignalHandler()

	// Add initial lister to sync rules and routes at start.
	initLister := &InitialLister{
		client: mgr.GetClient(),
		log:    log,
	}
	err = mgr.Add(initLister)
	if err != nil {
		log.Error(err, "add initial lister to the manager")
	}

	//
	if _, err := NewController(ctx, mgr, log); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Start the Manager.
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}

// InitialLister is a Runnable implementatin to access existing objects
// before handling any event with Reconcile method.
type InitialLister struct {
	log    logr.Logger
	client client.Client
}

func (i *InitialLister) Start(ctx context.Context) error {
	cl := i.client

	// List VMs, Pods, CRDs before starting manager.
	vms := v1alpha2.VirtualMachineList{}
	err := cl.List(ctx, &vms)
	if err != nil {
		i.log.Error(err, "list VMs")
		return err
	}
	log.Info(fmt.Sprintf("List returns %d VMs", len(vms.Items)))
	for _, vm := range vms.Items {
		i.log.Info(fmt.Sprintf("observe VM %s/%s at start", vm.GetNamespace(), vm.GetName()))
	}

	pods := corev1.PodList{}
	err = cl.List(ctx, &pods, client.InNamespace(""))
	if err != nil {
		i.log.Error(err, "list Pods")
		return err
	}
	log.Info(fmt.Sprintf("List returns %d Pods", len(pods.Items)))
	for _, pod := range pods.Items {
		i.log.Info(fmt.Sprintf("observe Pod %s/%s at start", pod.GetNamespace(), pod.GetName()))
	}

	crds := extv1.CustomResourceDefinitionList{}
	err = cl.List(ctx, &crds, client.InNamespace(""))
	if err != nil {
		i.log.Error(err, "list Pods")
		return err
	}
	log.Info(fmt.Sprintf("List returns %d CRDs", len(crds.Items)))
	for _, crd := range crds.Items {
		i.log.Info(fmt.Sprintf("observe CRD %s/%s at start", crd.GetNamespace(), crd.GetName()))
	}

	i.log.Info("Initial listing done, proceed to manager Start")
	return nil
}

const (
	controllerName = "test-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
) (controller.Controller, error) {
	reconciler := &VMReconciler{
		Client:   mgr.GetClient(),
		Cache:    mgr.GetCache(),
		Recorder: mgr.GetEventRecorderFor(controllerName),
		Scheme:   mgr.GetScheme(),
		Log:      log,
	}

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}

	if err = SetupWatches(ctx, mgr, c, log); err != nil {
		return nil, err
	}

	if err = SetupWebhooks(ctx, mgr, reconciler); err != nil {
		return nil, err
	}

	log.Info("Initialized controller with test watches")
	return c, nil
}

// SetupWatches subscripts controller to Pods, CRDs and DVP VMs.
func SetupWatches(ctx context.Context, mgr manager.Manager, ctr controller.Controller, log logr.Logger) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{}), &handler.EnqueueRequestForObject{},
		// if err := ctr.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info(fmt.Sprintf("Got CREATE event for VM %s/%s gvk %v", e.Object.GetNamespace(), e.Object.GetName(), e.Object.GetObjectKind().GroupVersionKind()))
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info(fmt.Sprintf("Got DELETE event for VM %s/%s gvk %v", e.Object.GetNamespace(), e.Object.GetName(), e.Object.GetObjectKind().GroupVersionKind()))
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				log.Info(fmt.Sprintf("Got UPDATE event for VM %s/%s gvk %v", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName(), e.ObjectNew.GetObjectKind().GroupVersionKind()))
				return true
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on DVP VMs: %w", err)
	}

	if err := ctr.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info(fmt.Sprintf("Got CREATE event for Pod %s/%s gvk %v", e.Object.GetNamespace(), e.Object.GetName(), e.Object.GetObjectKind().GroupVersionKind()))
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info(fmt.Sprintf("Got DELETE event for Pod %s/%s gvk %v", e.Object.GetNamespace(), e.Object.GetName(), e.Object.GetObjectKind().GroupVersionKind()))
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				log.Info(fmt.Sprintf("Got UPDATE event for Pod %s/%s gvk %v", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName(), e.ObjectNew.GetObjectKind().GroupVersionKind()))
				return true
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on Pods: %w", err)
	}

	if err := ctr.Watch(source.Kind(mgr.GetCache(), &extv1.CustomResourceDefinition{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				log.Info(fmt.Sprintf("Got CREATE event for CRD %s/%s gvk %v", e.Object.GetNamespace(), e.Object.GetName(), e.Object.GetObjectKind().GroupVersionKind()))
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				log.Info(fmt.Sprintf("Got DELETE event for CRD %s/%s gvk %v", e.Object.GetNamespace(), e.Object.GetName(), e.Object.GetObjectKind().GroupVersionKind()))
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				log.Info(fmt.Sprintf("Got UPDATE event for CRD %s/%s gvk %v", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName(), e.ObjectNew.GetObjectKind().GroupVersionKind()))
				return true
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on CRDs: %w", err)
	}

	return nil
}

func SetupWebhooks(ctx context.Context, mgr manager.Manager, validator admission.CustomValidator) error {
	return builder.WebhookManagedBy(mgr).
		For(&virtv1.VirtualMachine{}).
		WithValidator(validator).
		Complete()
}

type VMReconciler struct {
	Client   client.Client
	Cache    cache.Cache
	Recorder record.EventRecorder
	Scheme   *apiruntime.Scheme
	Log      logr.Logger
}

func (r *VMReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.Log.Info(fmt.Sprintf("Got request for %s", req.String()))
	return reconcile.Result{}, nil
}

func (r *VMReconciler) ValidateCreate(ctx context.Context, obj apiruntime.Object) (admission.Warnings, error) {
	vm, ok := obj.(*virtv1.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	}

	warnings := admission.Warnings{
		fmt.Sprintf("Validate new VM %s is OK, got kind %s, apiVersion %s", vm.GetName(), vm.GetObjectKind(), vm.APIVersion),
	}
	return warnings, nil
}

func (r *VMReconciler) ValidateUpdate(ctx context.Context, _, newObj apiruntime.Object) (admission.Warnings, error) {
	vm, ok := newObj.(*virtv1.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", newObj)
	}

	warnings := admission.Warnings{
		fmt.Sprintf("Validate updated VM %s is OK, got kind %s, apiVersion %s", vm.GetName(), vm.GetObjectKind(), vm.APIVersion),
	}
	return warnings, nil
}

func (v *VMReconciler) ValidateDelete(_ context.Context, obj apiruntime.Object) (admission.Warnings, error) {
	vm, ok := obj.(*virtv1.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a deleted VirtualMachine but got a %T", obj)
	}

	warnings := admission.Warnings{
		fmt.Sprintf("Validate deleted VM %s is OK, got kind %s, apiVersion %s", vm.GetName(), vm.GetObjectKind(), vm.APIVersion),
	}
	return warnings, nil
}
