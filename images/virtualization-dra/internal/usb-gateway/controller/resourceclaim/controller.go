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

package resourceclaim

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	vdraapi "github.com/deckhouse/virtualization-dra/api"
	"github.com/deckhouse/virtualization-dra/internal/common"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/informer"
	"github.com/deckhouse/virtualization-dra/internal/usbip"
)

const controllerName = "resourceclaim-controller"
const finalizer = "virtualization.deckhouse.io/usb-gateway"

var (
	keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

func controllerKeyFunc(namespace, name string) string {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}.String()
}

type Controller struct {
	nodeName             string
	podIP                net.IP
	usbipPort            int
	client               kubernetes.Interface
	resourceClaimIndexer cache.Indexer
	resourceSliceIndexer cache.Indexer
	nodeIndexer          cache.Indexer
	podIndexer           cache.Indexer
	usbIP                usbip.Interface
	queue                workqueue.TypedRateLimitingInterface[string]
	log                  *slog.Logger
	hasSynced            cache.InformerSynced
}

func NewController(nodeName string, podIP net.IP, usbipPort int, client kubernetes.Interface, resourceClaimInformer, resourceSliceInformer, nodeInformer, podInformer cache.SharedIndexInformer, usbIP usbip.Interface) (*Controller, error) {
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: controllerName},
	)

	c := &Controller{
		nodeName:             nodeName,
		podIP:                podIP,
		usbipPort:            usbipPort,
		client:               client,
		resourceClaimIndexer: resourceClaimInformer.GetIndexer(),
		resourceSliceIndexer: resourceSliceInformer.GetIndexer(),
		nodeIndexer:          nodeInformer.GetIndexer(),
		podIndexer:           podInformer.GetIndexer(),
		usbIP:                usbIP,
		queue:                queue,
		log:                  slog.With(slog.String("controller", controllerName)),
	}

	_, err := resourceClaimInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addResourceClaim,
		UpdateFunc: c.updateResourceClaim,
		DeleteFunc: c.deleteResourceClaim,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add event handler to resourceclaim informer: %w", err)
	}

	c.hasSynced = func() bool {
		return resourceClaimInformer.HasSynced() && nodeInformer.HasSynced() && podInformer.HasSynced() && resourceSliceInformer.HasSynced()
	}

	return c, nil
}

func (c *Controller) addResourceClaim(obj interface{}) {
	if rc, ok := obj.(*resourcev1beta1.ResourceClaim); ok {
		c.enqueueResourceClaim(rc)
	}
}

func (c *Controller) deleteResourceClaim(obj interface{}) {
	if rc, ok := obj.(*resourcev1beta1.ResourceClaim); ok {
		c.enqueueResourceClaim(rc)
	}
}

func (c *Controller) updateResourceClaim(_, newObj interface{}) {
	newRC, ok := newObj.(*resourcev1beta1.ResourceClaim)
	if !ok {
		return
	}

	if newRC.Status.Allocation != nil {
		c.enqueueResourceClaim(newRC)
	}
}

func (c *Controller) enqueueResourceClaim(rc *resourcev1beta1.ResourceClaim) {
	key, err := keyFunc(rc)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %w", rc, err))
		return
	}
	c.queueAdd(key)
}

func (c *Controller) queueAdd(key string) {
	c.queue.Add(key)
}

func (c *Controller) queueAfterAdd(key string, duration time.Duration) {
	c.queue.AddAfter(key, duration)
}

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.log.Info("Starting controller")
	defer c.log.Info("Shutting down controller")

	if !cache.WaitForCacheSync(ctx.Done(), c.hasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.log.Info("Starting workers controller")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.worker, time.Second)
	}

	<-ctx.Done()
	return nil
}

func (c *Controller) worker(ctx context.Context) {
	workFunc := func(ctx context.Context) bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.sync(key); err != nil {
			c.log.Error("re-enqueuing", slog.String("key", key), slog.Any("err", err))
			c.queue.AddRateLimited(key)
		} else {
			c.log.Info(fmt.Sprintf("processed ResourceClaim %v", key))
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
	log := c.log.With("key", key)
	log.Info("syncing resource claim")

	rc, err := c.getResourceClaim(key)
	if err != nil {
		return err
	}
	if rc == nil {
		return nil
	}
	if !rc.GetDeletionTimestamp().IsZero() {
		return c.handleDelete(rc)
	}

	pod, err := c.getReservedFor(rc)
	if err != nil {
		return err
	}
	if pod == nil {
		c.log.Info("no reserved pod found for resource claim, re-enqueue after 10s")
		c.queueAfterAdd(key, time.Second*10)
		return nil
	}

	myAllocationDevices, otherAllocationDevices, err := c.getAllocationDevices(rc)
	if err != nil {
		return err
	}

	onMyNode := c.podOnMyNode(pod)
	shouldShare := !onMyNode && len(myAllocationDevices) > 0
	shouldAttach := onMyNode && len(otherAllocationDevices) > 0

	switch {
	case shouldShare:
		log.Info("sharing usb to other node")
		if err = c.handleServer(rc, myAllocationDevices); err != nil {
			return fmt.Errorf("failed to handle server: %w", err)
		}
	case shouldAttach:
		log.Info("attaching usb to my node")
		if err = c.handleClient(rc, otherAllocationDevices); err != nil {
			return fmt.Errorf("failed to handle client: %w", err)
		}

	}

	return nil
}

// TODO: handle detach too
func (c *Controller) handleDelete(rc *resourcev1beta1.ResourceClaim) error {
	if c.allUnBound(rc) {
		return c.removeFinalizer(rc)
	}

	myAllocationDevices, _, err := c.getAllocationDevices(rc)
	if err != nil {
		return err
	}

	myAllocationDevicesByName := make(map[string]resourcev1beta1.Device)
	for _, device := range myAllocationDevices {
		myAllocationDevicesByName[device.Name] = device
	}

	shouldUpdate := false

	for i := range rc.Status.Devices {
		allocatedDeviceStatus := &rc.Status.Devices[i]

		usbGatewayStatus := vdraapi.FromData(allocatedDeviceStatus.Data)
		if usbGatewayStatus == nil {
			continue
		}
		if !usbGatewayStatus.Bound {
			continue
		}

		device, ok := myAllocationDevicesByName[allocatedDeviceStatus.Device]
		if !ok {
			continue
		}

		busID := ""
		if attr, ok := device.Basic.Attributes["busID"]; ok && attr.StringValue != nil {
			busID = *attr.StringValue
		} else {
			continue
		}

		// TODO: device can be added to other resource claims. Not supported yet.
		c.log.Info("unbinding usb")
		if err = c.usbIP.Unbind(busID); err != nil {
			return fmt.Errorf("failed to unbind usb: %w", err)
		}

		usbGatewayStatus.Bound = false
		allocatedDeviceStatus.Data.Object = usbGatewayStatus
		shouldUpdate = true
	}

	if shouldUpdate {
		_, err = c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).UpdateStatus(context.Background(), rc, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update resource claim status: %w", err)
		}
	}

	return nil
}

func (c *Controller) allUnBound(rc *resourcev1beta1.ResourceClaim) bool {
	for _, deviceStatus := range rc.Status.Devices {
		usbGatewayStatus := vdraapi.FromData(deviceStatus.Data)
		if usbGatewayStatus == nil {
			continue
		}
		if usbGatewayStatus.Bound {
			return false
		}
	}

	return true
}

func (c *Controller) addFinalizer(rc *resourcev1beta1.ResourceClaim) error {
	var newFinalizers []string
	for _, fin := range rc.GetFinalizers() {
		if fin == finalizer {
			return nil
		}
		newFinalizers = append(newFinalizers, fin)
	}
	newFinalizers = append(newFinalizers, finalizer)
	rc.SetFinalizers(newFinalizers)
	_, err := c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).Update(context.Background(), rc, metav1.UpdateOptions{})
	return err
}

func (c *Controller) removeFinalizer(rc *resourcev1beta1.ResourceClaim) error {
	var newFinalizers []string
	for _, fin := range rc.GetFinalizers() {
		if fin == finalizer {
			continue
		}
		newFinalizers = append(newFinalizers, fin)
	}
	if len(newFinalizers) == len(rc.GetFinalizers()) {
		return nil
	}

	rc.SetFinalizers(newFinalizers)
	_, err := c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).Update(context.Background(), rc, metav1.UpdateOptions{})
	return err
}

func (c *Controller) handleServer(rc *resourcev1beta1.ResourceClaim, myAllocationDevices []resourcev1beta1.Device) error {
	indexAllocDevice := make(map[string]int)
	for i, allocDeviceStatus := range rc.Status.Devices {
		indexAllocDevice[allocDeviceStatus.Device] = i
	}

	shouldUpdate := false

	for _, device := range myAllocationDevices {
		if device.Basic == nil {
			continue
		}

		index, ok := indexAllocDevice[device.Name]
		if !ok {
			continue
		}

		allocDeviceStatus := &rc.Status.Devices[index]

		usbGatewayStatus := vdraapi.FromData(allocDeviceStatus.Data)

		targetIPAlreadySet := usbGatewayStatus != nil && usbGatewayStatus.TargetIP != ""
		if targetIPAlreadySet {
			continue
		}
		usbGatewayStatus = &vdraapi.USBGatewayStatus{}

		busID := ""
		if attr, ok := device.Basic.Attributes["busID"]; ok && attr.StringValue != nil {
			busID = *attr.StringValue
		} else {
			continue
		}

		bound, err := c.usbIP.IsBound(busID)
		if err != nil {
			return fmt.Errorf("failed to check if usb is bound: %w", err)
		}

		if !bound {
			if err = c.usbIP.Bind(busID); err != nil {
				return fmt.Errorf("failed to bind usb: %w", err)
			}
		}

		usbGatewayStatus.TargetIP = c.podIP.String()
		usbGatewayStatus.TargetPort = c.usbipPort
		usbGatewayStatus.Bound = true

		if allocDeviceStatus.Data == nil {
			allocDeviceStatus.Data = &runtime.RawExtension{}
		}
		allocDeviceStatus.Data.Object = usbGatewayStatus
		shouldUpdate = true
	}

	if shouldUpdate {
		err := c.addFinalizer(rc)
		if err != nil {
			return err
		}
		_, err = c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).UpdateStatus(context.Background(), rc, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update resource claim status: %w", err)
		}
	}

	return nil
}

func (c *Controller) handleClient(rc *resourcev1beta1.ResourceClaim, otherAllocationDevices []resourcev1beta1.Device) error {
	indexAllocDevice := make(map[string]int)
	for i, allocDeviceStatus := range rc.Status.Devices {
		indexAllocDevice[allocDeviceStatus.Device] = i
	}

	shouldUpdate := false

	for _, device := range otherAllocationDevices {
		if device.Basic == nil {
			continue
		}
		index, ok := indexAllocDevice[device.Name]
		if !ok {
			continue
		}
		allocDeviceStatus := &rc.Status.Devices[index]
		usbGatewayStatus := vdraapi.FromData(allocDeviceStatus.Data)
		if usbGatewayStatus == nil {
			continue
		}
		if !usbGatewayStatus.Bound {
			continue
		}
		if usbGatewayStatus.Attached {
			continue
		}
		busID := ""
		if attr, ok := device.Basic.Attributes["busID"]; ok && attr.StringValue != nil {
			busID = *attr.StringValue
		} else {
			continue
		}

		err := c.usbIP.Attach(usbGatewayStatus.TargetIP, busID, usbGatewayStatus.TargetPort)
		if err != nil {
			return fmt.Errorf("failed to attach usb: %w", err)
		}

		usbGatewayStatus.Attached = true
		if allocDeviceStatus.Data == nil {
			allocDeviceStatus.Data = &runtime.RawExtension{}
		}
		allocDeviceStatus.Data.Object = usbGatewayStatus
		shouldUpdate = true
	}

	if shouldUpdate {
		_, err := c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).UpdateStatus(context.Background(), rc, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update resource claim status: %w", err)
		}
	}

	return nil
}

func (c *Controller) getResourceClaim(key string) (*resourcev1beta1.ResourceClaim, error) {
	obj, exists, err := c.resourceClaimIndexer.GetByKey(key)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get resourceclaim: %w", err)
	}
	if !exists {
		return nil, nil
	}

	rc, ok := obj.(*resourcev1beta1.ResourceClaim)
	if !ok {
		return nil, fmt.Errorf("unexpected type of resourceclaim: %T", obj)
	}

	return rc.DeepCopy(), nil
}

func (c *Controller) getPod(name, namespace string) (*corev1.Pod, error) {
	obj, exists, err := c.podIndexer.GetByKey(controllerKeyFunc(namespace, name))
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("unexpected type of pod: %T", obj)
	}

	return pod.DeepCopy(), nil
}

func (c *Controller) getVirtualizationDraResourceSlices() ([]resourcev1beta1.ResourceSlice, error) {
	slicesObj, err := c.resourceSliceIndexer.ByIndex(informer.DriverIndex, common.VirtualizationDraPluginName)
	if err != nil {
		return nil, err
	}
	var slices []resourcev1beta1.ResourceSlice
	for _, obj := range slicesObj {
		slice, ok := obj.(resourcev1beta1.ResourceSlice)
		if !ok {
			return nil, fmt.Errorf("unexpected type of resource slice: %T", obj)
		}
		slices = append(slices, *slice.DeepCopy())
	}
	return slices, nil
}

// TODO: refactor me. only one pod supports now
func (c *Controller) getReservedFor(rc *resourcev1beta1.ResourceClaim) (*corev1.Pod, error) {
	for _, rFor := range rc.Status.ReservedFor {
		if rFor.Resource == "pods" {
			pod, err := c.getPod(rFor.Name, rc.Namespace)
			if err != nil {
				return nil, err
			}
			if pod == nil || pod.GetUID() != rFor.UID {
				return nil, nil
			}
			return pod, nil
		}
	}
	return nil, nil
}

func (c *Controller) podOnMyNode(pod *corev1.Pod) bool {
	return pod.Spec.NodeName == c.nodeName
}

func (c *Controller) getAllocationDevices(rc *resourcev1beta1.ResourceClaim) ([]resourcev1beta1.Device, []resourcev1beta1.Device, error) {
	if rc.Status.Allocation == nil {
		return nil, nil, fmt.Errorf("resource claim %s/%s has no allocation", rc.Namespace, rc.Name)
	}

	virtualizationDraSlices, err := c.getVirtualizationDraResourceSlices()
	if err != nil {
		return nil, nil, err
	}

	byPoolSlices := make(map[string][]resourcev1beta1.ResourceSlice)
	for _, slice := range virtualizationDraSlices {
		byPoolSlices[slice.Spec.Pool.Name] = append(byPoolSlices[slice.Spec.Pool.Name], slice)
	}

	allocResultsByPool := make(map[string]map[string]resourcev1beta1.DeviceRequestAllocationResult)

	for _, status := range rc.Status.Allocation.Devices.Results {
		if status.Driver != common.VirtualizationDraPluginName {
			continue
		}
		// now, driver virtualization-dra supports only usb, but we can add more devices later
		// so we need to check if the device is usb
		if strings.HasPrefix(status.Device, "usb") {
			continue
		}

		allocResultsByPool[status.Pool][status.Device] = status
	}

	var myDevices []resourcev1beta1.Device
	var otherDevices []resourcev1beta1.Device

	for pool, allocResultsByDevice := range allocResultsByPool {
		slices, ok := byPoolSlices[pool]
		if !ok {
			return nil, nil, fmt.Errorf("no resource slices found for pool %s", pool)
		}

		for _, slice := range slices {
			for _, device := range slice.Spec.Devices {
				allocResult, ok := allocResultsByDevice[device.Name]
				if !ok {
					continue
				}

				// virtualization-dra creates slices with pool name by node name
				if allocResult.Pool == c.nodeName {
					myDevices = append(myDevices, device)
				} else {
					otherDevices = append(otherDevices, device)
				}
			}
		}
	}

	return myDevices, otherDevices, nil
}
