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
	"net"
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
	cache2 "vm-route-forge/internal/cache"
	"vm-route-forge/internal/netlinkmanager"
)

const controllerName = "routeController"

var (
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

type Runnable interface {
	Run(ctx context.Context) error
}

func NewRouteController(
	vmInformer virtinformers.VirtualMachineInformer,
	cnInformer ciliumv2Informers.CiliumNodeInformer,
	netlinkMgr *netlinkmanager.Manager,
	sharedCache cache2.Cache,
	cidrs []*net.IPNet,
	logger logr.Logger,
) (*Controller, error) {

	queue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: controllerName})
	log := logger.WithValues("controller", controllerName)
	routeController := &Controller{
		queue:          queue,
		hostReconciler: NewHostController(queue, cidrs, sharedCache, log),
		cache:          sharedCache,
		netlinkMgr:     netlinkMgr,
		log:            log,
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
	routeController.hasSynced = func() bool {
		return vmInformer.Informer().HasSynced() && cnInformer.Informer().HasSynced()
	}

	return routeController, nil
}

type Controller struct {
	vmIndexer      cache.Indexer
	vmLister       virtlisters.VirtualMachineLister
	hostReconciler Runnable
	hasSynced      cache.InformerSynced
	queue          workqueue.RateLimitingInterface
	cache          cache2.Cache
	netlinkMgr     *netlinkmanager.Manager
	log            logr.Logger
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
	c.queue.Add(key)
}

func (c *Controller) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.log.Info("Starting route controller")
	defer c.log.Info("Shutting down route controller")

	if !cache.WaitForNamedCacheSync(controllerName, ctx.Done(), c.hasSynced) {
		c.log.Error(fmt.Errorf("cache is not synced"), "Controller will be stopped", "controller", controllerName)
		return
	}

	c.log.Info("Starting workers of route controller")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.worker, time.Second)
	}
	c.log.Info("Starting localhost route controller")
	errCh := make(chan error)
	go func() {
		errCh <- c.hostReconciler.Run(ctx)
	}()

	for {
		select {
		case err := <-errCh:
			if err != nil {
				c.log.Error(err, "host reconciliation failed")
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Controller) worker(ctx context.Context) {
	workFunc := func(ctx context.Context) bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.sync(ctx, key.(string)); err != nil {
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

func (c *Controller) sync(ctx context.Context, key string) error {
	obj, exists, err := c.vmIndexer.GetByKey(key)
	if err != nil {
		return err
	}
	ns, name, _ := strings.Cut(key, "/")
	k := types.NamespacedName{Name: name, Namespace: ns}
	if !exists {
		c.netlinkMgr.DeleteRoute(k, "")
		return nil
	}
	originalVM := obj.(*v1alpha2.VirtualMachine)
	vm := originalVM.DeepCopy()
	log := c.log.WithValues("virtualmachine", key)
	log.Info("Started processing vm")

	if vm.GetDeletionTimestamp() != nil {
		c.netlinkMgr.DeleteRoute(k, vm.Status.IPAddress)
		return nil
	}

	c.netlinkMgr.UpdateRoute(ctx, vm)
	return nil
}
