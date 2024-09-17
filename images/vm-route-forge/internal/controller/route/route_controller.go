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

package route

import (
	"context"
	"fmt"
	"strings"
	"time"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	ciliumv2Informers "github.com/cilium/cilium/pkg/k8s/client/informers/externalversions/cilium.io/v2"
	"github.com/cilium/cilium/pkg/node/addressing"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	virtinformers "github.com/deckhouse/virtualization/api/client/generated/informers/externalversions/core/v1alpha2"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"vm-route-forge/internal/netlinkmanager"
)

const controllerName = "routeController"

var (
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

type Runnable interface {
	Run(ctx context.Context) error
}

func NewController(
	vmInformer virtinformers.VirtualMachineInformer,
	cnInformer ciliumv2Informers.CiliumNodeInformer,
	routeWatcher Watcher,
	netlinkMgr *netlinkmanager.Manager,
	logger logr.Logger,
) (*Controller, error) {

	queue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: controllerName})
	log := logger.WithValues("controller", controllerName)
	routeController := &Controller{
		queue:        queue,
		routeWatcher: routeWatcher,
		netlinkMgr:   netlinkMgr,
		log:          log,
	}

	_, err := vmInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    routeController.addVirtualMachine,
		DeleteFunc: routeController.deleteVirtualMachine,
		UpdateFunc: routeController.updateVirtualMachine,
	})
	if err != nil {
		return nil, err
	}
	_, err = cnInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    routeController.addCiliumNode,
		DeleteFunc: routeController.deleteCiliumNode,
		UpdateFunc: routeController.updateCiliumNode,
	})
	if err != nil {
		return nil, err
	}
	routeController.vmIndexer = vmInformer.Informer().GetIndexer()
	routeController.vmLister = vmInformer.Lister()
	routeController.cnIndexer = cnInformer.Informer().GetIndexer()
	routeController.hasSynced = func() bool {
		return vmInformer.Informer().HasSynced() && cnInformer.Informer().HasSynced()
	}

	return routeController, nil
}

type Controller struct {
	vmIndexer    cache.Indexer
	cnIndexer    cache.Indexer
	vmLister     virtlisters.VirtualMachineLister
	routeWatcher Watcher
	hasSynced    cache.InformerSynced
	queue        workqueue.RateLimitingInterface
	netlinkMgr   *netlinkmanager.Manager
	log          logr.Logger
}

func (c *Controller) addVirtualMachine(obj interface{}) {
	vm, ok := obj.(*v1alpha2.VirtualMachine)
	if !ok {
		return
	}
	c.enqueueVirtualMachine(vm)
}

func (c *Controller) deleteVirtualMachine(obj interface{}) {
	vm, ok := obj.(*v1alpha2.VirtualMachine)
	if !ok {
		return
	}
	c.enqueueVirtualMachine(vm)
}
func (c *Controller) updateVirtualMachine(oldObj interface{}, newObj interface{}) {
	oldVm, ok := oldObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return
	}
	newVm, ok := newObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return
	}
	if oldVm.Status.IPAddress != newVm.Status.IPAddress || oldVm.Status.Node != newVm.Status.Node {
		c.enqueueVirtualMachine(newVm)
	}
}

func (c *Controller) addCiliumNode(_ interface{}) {
	// Do nothing
}

func (c *Controller) deleteCiliumNode(obj interface{}) {
	node, ok := obj.(*ciliumv2.CiliumNode)
	if !ok {
		return
	}
	vms, err := c.vmLister.List(labels.Everything())
	if err != nil {
		c.log.Error(err, "failed to list virtual machines")
		return
	}

	for _, vm := range vms {
		if vm.Status.Node == node.Name {
			c.enqueueVirtualMachine(vm)
		}
	}

}
func (c *Controller) updateCiliumNode(oldObj interface{}, newObj interface{}) {
	oldNode, ok := oldObj.(*ciliumv2.CiliumNode)
	if !ok {
		return
	}
	newNode, ok := newObj.(*ciliumv2.CiliumNode)
	if !ok {
		return
	}

	oldIP := c.getCiliumInternalIP(oldNode)
	newIP := c.getCiliumInternalIP(newNode)

	if oldIP == newIP {
		return
	}
	vms, err := c.vmLister.List(labels.Everything())
	if err != nil {
		c.log.Error(err, "failed to list virtual machines")
		return
	}

	for _, vm := range vms {
		if vm.Status.Node == newNode.Name {
			c.enqueueVirtualMachine(vm)
		}
	}
}

func (c *Controller) getCiliumInternalIP(node *ciliumv2.CiliumNode) string {
	for _, addr := range node.Spec.Addresses {
		if addr.Type == addressing.NodeCiliumInternalIP {
			return addr.IP
		}
	}
	return ""
}

func (c *Controller) enqueueVirtualMachine(vm *v1alpha2.VirtualMachine) {
	key, err := KeyFunc(vm)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %w", vm, err))
		return
	}
	c.queueAdd(key)
}

func (c *Controller) queueAdd(key string) {
	c.queue.Add(key)
}

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.log.Info("Starting route controller")
	defer c.log.Info("Shutting down route controller")

	if !cache.WaitForNamedCacheSync(controllerName, newCtx.Done(), c.hasSynced) {
		return fmt.Errorf("cache is not synced")
	}

	if err := c.netlinkMgr.SyncRules(); err != nil {
		return fmt.Errorf("failed to synchronize routing rules at start: %w", err)
	}

	go func() {
		// AddSubnetRoutesToBlackHole will be executed every minute until context canceled.
		wait.UntilWithContext(newCtx, func(_ context.Context) {
			if err := c.netlinkMgr.AddSubnetsRoutesToBlackHole(); err != nil {
				c.log.Error(err, "Failed to add blackhole routes for subnets.")
			}
		}, time.Minute)
	}()

	c.log.Info("Starting workers of route controller")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(newCtx, c.worker, time.Second)
	}
	c.log.Info("Starting localhost route controller")

	go func() {
		for key := range c.routeWatcher.ResultChannel() {
			c.queueAdd(key.String())
		}
		cancel()
	}()

	<-newCtx.Done()
	return nil
}

func (c *Controller) worker(ctx context.Context) {
	workFunc := func(ctx context.Context) bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.sync(key.(string)); err != nil {
			c.log.Error(err, fmt.Sprintf("re-enqueuing VirtualMachine %v", key))
			c.queue.AddRateLimited(key)
		} else {
			c.log.Info(fmt.Sprintf("processed VirtualMachine %v", key))
			c.queue.Forget(key)
		}
		return false
	}
	for {
		quit := workFunc(ctx)

		if quit {
			return
		}
	}
}

func (c *Controller) sync(key string) error {
	log := c.log.WithValues("virtualmachine", key)
	log.Info("Started processing vm")

	obj, exists, err := c.vmIndexer.GetByKey(key)
	if err != nil {
		return err
	}
	ns, name, _ := strings.Cut(key, "/")
	k := types.NamespacedName{Name: name, Namespace: ns}
	if !exists {
		if err = c.netlinkMgr.DeleteRoute(k, ""); err != nil {
			return fmt.Errorf("failed to delete route: %w", err)
		}
		return nil
	}
	originalVM := obj.(*v1alpha2.VirtualMachine)
	vm := originalVM.DeepCopy()

	if vm.GetDeletionTimestamp() != nil {
		if err = c.netlinkMgr.DeleteRoute(k, vm.Status.IPAddress); err != nil {
			return fmt.Errorf("failed to delete route: %w", err)
		}
		return nil
	}

	if vm.Status.Node == "" {
		log.Info("Node is empty, re-enqueuing after 60s.")
		c.queue.AddAfter(k.String(), 60*time.Second)
		return nil
	}

	// Retrieve a Cilium Node by VMs node name.
	var ciliumNode *ciliumv2.CiliumNode
	obj, exists, err = c.cnIndexer.GetByKey(vm.Status.Node)
	if err != nil {
		return fmt.Errorf("failed to get cilium node for vm: %w", err)
	}
	if exists {
		ciliumNode = obj.(*ciliumv2.CiliumNode)
	}

	if err = c.netlinkMgr.UpdateRoute(vm, ciliumNode); err != nil {
		return fmt.Errorf("failed to update route: %w", err)
	}
	return nil
}
