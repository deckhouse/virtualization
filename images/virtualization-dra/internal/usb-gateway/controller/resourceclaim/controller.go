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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/strings/slices"

	vdraapi "github.com/deckhouse/virtualization-dra/api/usbgateway/v1alpha1"
	"github.com/deckhouse/virtualization-dra/internal/common"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/informer"
	"github.com/deckhouse/virtualization-dra/internal/usbip"
	"github.com/deckhouse/virtualization-dra/pkg/controller"
	"github.com/deckhouse/virtualization-dra/pkg/patch"
)

const controllerName = "resourceclaim-controller"
const finalizer = "virtualization.deckhouse.io/usb-gateway"

type Controller struct {
	nodeName             string
	usbipdAddr           string
	usbipdPort           int
	client               kubernetes.Interface
	resourceClaimIndexer cache.Indexer
	resourceSliceIndexer cache.Indexer
	nodeIndexer          cache.Indexer
	podIndexer           cache.Indexer
	usbIP                usbip.Interface
	queue                workqueue.TypedRateLimitingInterface[string]
	log                  *slog.Logger
	hasSynced            cache.InformerSynced
	recordManager        *recordManager
}

func NewController(nodeName, usbipdAddr string, usbipdPort int, client kubernetes.Interface, resourceClaimInformer, resourceSliceInformer, nodeInformer, podInformer cache.SharedIndexInformer, usbIP usbip.Interface) (*Controller, error) {
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: controllerName},
	)

	recordManager, err := newRecordManager(DefaultRecordStateDir, usbIP)
	if err != nil {
		return nil, err
	}

	c := &Controller{
		nodeName:             nodeName,
		usbipdAddr:           usbipdAddr,
		usbipdPort:           usbipdPort,
		client:               client,
		resourceClaimIndexer: resourceClaimInformer.GetIndexer(),
		resourceSliceIndexer: resourceSliceInformer.GetIndexer(),
		nodeIndexer:          nodeInformer.GetIndexer(),
		podIndexer:           podInformer.GetIndexer(),
		usbIP:                usbIP,
		queue:                queue,
		log:                  slog.With(slog.String("controller", controllerName)),
		recordManager:        recordManager,
	}

	_, err = resourceClaimInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addResourceClaim,
		UpdateFunc: c.updateResourceClaim,
		DeleteFunc: c.deleteResourceClaim,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add event handler to resourceclaim informer: %w", err)
	}

	_, err = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.deletePod,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add event handler to pod informer: %w", err)
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

func (c *Controller) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	for _, status := range pod.Status.ResourceClaimStatuses {
		if status.ResourceClaimName != nil {
			c.queueAdd(fmt.Sprintf("%s/%s", pod.Namespace, *status.ResourceClaimName))
		}
	}
}

func (c *Controller) enqueueResourceClaim(rc *resourcev1beta1.ResourceClaim) {
	key, err := controller.ObjectKeyFunc(rc)
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

func (c *Controller) Queue() workqueue.TypedRateLimitingInterface[string] {
	return c.queue
}

func (c *Controller) HasSynced() bool {
	return c.hasSynced()
}

func (c *Controller) Logger() *slog.Logger {
	return c.log
}

func (c *Controller) Sync(ctx context.Context, key string) error {
	log := c.log.With("key", key)
	log.Info("syncing resource claim")

	rc, err := c.getMyResourceClaim(key)
	if err != nil {
		return err
	}
	if rc == nil {
		return nil
	}

	resourceClaimDeleting := !rc.GetDeletionTimestamp().IsZero()

	pod, err := c.getReservedFor(rc)
	if err != nil {
		return err
	}

	podExist := pod != nil

	if !podExist && !resourceClaimDeleting {
		c.log.Info("no reserved pod found for resource claim, re-enqueue after 10s")
		c.queueAfterAdd(key, time.Second*10)
		return nil
	}

	onMyNode := podExist && c.podOnMyNode(pod)
	if onMyNode && c.podFinished(pod) {
		log.Info("Pod finished, detach all usb devices for this pod",
			slog.String("podName", pod.Name),
			slog.String("podNamespace", pod.Namespace),
		)
		return c.handleClientPodFinished(pod)
	}

	if resourceClaimDeleting {
		if podExist {
			log.Info("Pod exists, waiting for pod to be deleted")
			c.queueAfterAdd(key, time.Second*10)
			return nil
		}
		log.Info("ResourceClaim is deleting, unbind all usb devices for this resource claim")
		return c.handleServerDeleteResourceClaim(rc)
	}

	// pod exists here
	log = log.With(slog.String("podName", pod.Name), slog.String("podNamespace", pod.Namespace))

	myAllocationDevices, otherAllocationDevices, err := c.getAllocationDevices(rc)
	if err != nil {
		return err
	}

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
		if err = c.handleClient(ctx, rc, otherAllocationDevices, pod); err != nil {
			return fmt.Errorf("failed to handle client: %w", err)
		}
	}

	return nil
}

func (c *Controller) podFinished(pod *corev1.Pod) bool {
	return !pod.GetDeletionTimestamp().IsZero() || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

// handle on client node, should detach all usb for this pod
func (c *Controller) handleClientPodFinished(pod *corev1.Pod) error {
	err := c.recordManager.Refresh()
	if err != nil {
		return fmt.Errorf("failed to Refresh record: %w", err)
	}

	ports := make(map[int]struct{})

	for _, entry := range c.recordManager.GetEntries() {
		if entry.PodUID == pod.UID {
			if _, ok := ports[entry.Port]; ok {
				continue
			}
			if err = c.usbIP.Detach(entry.Port); err != nil {
				return fmt.Errorf("failed to detach usb: %w", err)
			}
			ports[entry.Port] = struct{}{}
		}
	}

	return c.removeFinalizerForPod(pod)
}

// handle on server node, should unbind usb
func (c *Controller) handleServerDeleteResourceClaim(rc *resourcev1beta1.ResourceClaim) error {
	infos, err := c.usbIP.GetBindInfo()
	if err != nil {
		return fmt.Errorf("failed to get used info: %w", err)
	}

	for _, deviceStatus := range rc.Status.Devices {
		usbGatewayStatus, err := vdraapi.FromData(deviceStatus.Data)
		if err != nil {
			return err
		}

		busID := usbGatewayStatus.BusID

		for _, info := range infos {
			if info.BusID == busID {
				if info.Bound {
					err = c.usbIP.Unbind(busID)
					if err != nil {
						return fmt.Errorf("failed to unbind usb: %w", err)
					}
				}
			}
		}
	}

	return c.removeFinalizerForResourceClaim(rc)
}

func (c *Controller) allUnBound(rc *resourcev1beta1.ResourceClaim) (bool, error) {
	for _, deviceStatus := range rc.Status.Devices {
		usbGatewayStatus, err := vdraapi.FromData(deviceStatus.Data)
		if err != nil {
			return false, err
		}
		if usbGatewayStatus == nil {
			continue
		}
		if usbGatewayStatus.Bound {
			return false, nil
		}
	}

	return true, nil
}

func (c *Controller) addFinalizerForResourceClaim(rc *resourcev1beta1.ResourceClaim) error {
	patchBytes, err := makeAddFinalizerPatch(rc)
	if err != nil {
		return err
	}
	if patchBytes != nil {
		_, err = c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).Patch(context.Background(), rc.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		return err
	}
	return nil
}

func (c *Controller) removeFinalizerForResourceClaim(rc *resourcev1beta1.ResourceClaim) error {
	patchBytes, err := makeRemoveFinalizerPatch(rc)
	if err != nil {
		return err
	}
	if patchBytes != nil {
		_, err = c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).Patch(context.Background(), rc.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		return err
	}
	return nil
}

func (c *Controller) addFinalizerForPod(pod *corev1.Pod) error {
	patchBytes, err := makeAddFinalizerPatch(pod)
	if err != nil {
		return err
	}
	if patchBytes != nil {
		_, err = c.client.CoreV1().Pods(pod.Namespace).Patch(context.Background(), pod.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		return err
	}
	return nil
}

func (c *Controller) removeFinalizerForPod(pod *corev1.Pod) error {
	patchBytes, err := makeRemoveFinalizerPatch(pod)
	if err != nil {
		return err
	}
	if patchBytes != nil {
		_, err = c.client.CoreV1().Pods(pod.Namespace).Patch(context.Background(), pod.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		return err
	}
	return nil
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
			var (
				driver string
				pool   string
			)
			if rc.Status.Allocation != nil {
				for _, result := range rc.Status.Allocation.Devices.Results {
					if result.Device == device.Name {
						driver = result.Driver
						pool = result.Pool
					}
				}
			}
			if driver == "" || pool == "" {
				return fmt.Errorf("device %s is not allocated, driver or pool is empty", device.Name)
			}

			rc.Status.Devices = append(rc.Status.Devices, resourcev1beta1.AllocatedDeviceStatus{
				Driver: driver,
				Pool:   pool,
				Device: device.Name,
			})
		}

		allocDeviceStatus := &rc.Status.Devices[index]

		usbGatewayStatus, err := vdraapi.FromData(allocDeviceStatus.Data)
		if err != nil {
			return err
		}

		targetIPAlreadySet := usbGatewayStatus != nil && usbGatewayStatus.RemoteIP != ""
		targetIPWrong := usbGatewayStatus != nil && usbGatewayStatus.RemoteIP != c.usbipdAddr

		if targetIPAlreadySet && !targetIPWrong {
			continue
		}

		usbGatewayStatus = &vdraapi.USBGatewayStatus{
			TypeMeta: metav1.TypeMeta{
				APIVersion: vdraapi.SchemeGroupVersion.String(),
				Kind:       vdraapi.USBGatewayStatusKind,
			},
		}

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

		usbGatewayStatus.RemoteIP = c.usbipdAddr
		usbGatewayStatus.RemotePort = c.usbipdPort
		usbGatewayStatus.Bound = true

		data, err := vdraapi.ToData(usbGatewayStatus)
		if err != nil {
			return err
		}

		allocDeviceStatus.Data = data
		shouldUpdate = true
	}

	if shouldUpdate {
		err := c.addFinalizerForResourceClaim(rc)
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

func (c *Controller) handleClient(ctx context.Context, rc *resourcev1beta1.ResourceClaim, otherAllocationDevices []resourcev1beta1.Device, pod *corev1.Pod) error {
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
		usbGatewayStatus, err := vdraapi.FromData(allocDeviceStatus.Data)
		if err != nil {
			return err
		}
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

		err = c.recordManager.Refresh()
		if err != nil {
			return fmt.Errorf("failed to Refresh record: %w", err)
		}

		if err = c.attach(ctx, busID, usbGatewayStatus, rc.UID, pod.UID); err != nil {
			return fmt.Errorf("failed to attach usb: %w", err)
		}

		allocDeviceStatus.Data, err = vdraapi.ToData(usbGatewayStatus)
		if err != nil {
			return err
		}
		shouldUpdate = true
	}

	if shouldUpdate {
		err := c.addFinalizerForPod(pod)
		if err != nil {
			return fmt.Errorf("failed to add finalizer for pod: %s/%s: %w", pod.Namespace, pod.Name, err)
		}

		_, err = c.client.ResourceV1beta1().ResourceClaims(rc.Namespace).UpdateStatus(context.Background(), rc, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update resource claim status: %w", err)
		}
	}

	return nil
}

func (c *Controller) attach(ctx context.Context, busID string, usbGatewayStatus *vdraapi.USBGatewayStatus, claimUID, podUID types.UID) error {
	entries := c.recordManager.GetEntries()
	for _, entry := range entries {
		if entry.BusID == busID {
			if entry.RemoteBusID != usbGatewayStatus.RemoteIP || entry.RemotePort != usbGatewayStatus.RemotePort {
				if err := c.detach(entry.Port); err != nil {
					return err
				}
			}
			// already attached
			return nil
		}
	}

	var err error
	var rhport int

	defer func() {
		if err != nil && rhport >= 0 {
			if err := c.detach(rhport); err != nil {
				c.log.Error("failed to detach usb", slog.String("error", err.Error()), slog.Int("port", rhport))
			}
		}
	}()

	rhport, err = c.usbIP.Attach(usbGatewayStatus.RemoteIP, busID, usbGatewayStatus.RemotePort)
	if err != nil {
		return fmt.Errorf("failed to attach usb: %w", err)
	}

	// command attach was successful, but we need to wait until usb is real attached
	var usedInfo *usbip.AttachInfo
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		c.log.Info("Get attach info for store localBusID")
		infos, err := c.usbIP.GetAttachInfo()
		if err != nil {
			c.log.Info("Failed to get used info", slog.String("error", err.Error()))
			return false, nil
		}
		for _, info := range infos {
			if info.Port == rhport {
				usedInfo = &info
				return true, nil
			}
		}
		c.log.Info("Usb are not attached yet")
		return false, nil
	})

	entry := Entry{
		Port:             rhport,
		RemotePort:       usbGatewayStatus.RemotePort,
		RemoteIP:         usbGatewayStatus.RemoteIP,
		RemoteBusID:      busID,
		BusID:            usedInfo.LocalBusID,
		ResourceClaimUID: claimUID,
		PodUID:           podUID,
	}

	if err = c.recordManager.AddEntry(entry); err != nil {
		return fmt.Errorf("failed to add entry: %w", err)
	}

	usbGatewayStatus.Attached = true
	usbGatewayStatus.BusID = usedInfo.LocalBusID

	return nil
}

func (c *Controller) detach(port int) error {
	if err := c.usbIP.Detach(port); err != nil {
		return fmt.Errorf("failed to detach usb: %w", err)
	}
	return nil
}

func (c *Controller) getMyResourceClaim(key string) (*resourcev1beta1.ResourceClaim, error) {
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

	if c.isMyResourceClaim(rc) {
		return rc.DeepCopy(), nil
	}

	return nil, nil
}

func (c *Controller) isMyResourceClaim(rc *resourcev1beta1.ResourceClaim) bool {
	if rc == nil {
		return false
	}
	if slices.Contains(rc.GetFinalizers(), finalizer) {
		return true
	}
	if rc.Status.Allocation == nil {
		return false
	}
	for _, status := range rc.Status.Allocation.Devices.Results {
		// now, driver virtualization-dra supports only usb, but we can add more devices later
		// so we need to check if the device is usb
		if status.Driver == common.VirtualizationDraPluginName && strings.HasPrefix(status.Device, "usb") {
			return true
		}
	}
	return false
}

func (c *Controller) getPod(name, namespace string) (*corev1.Pod, error) {
	obj, exists, err := c.podIndexer.GetByKey(controller.KeyFunc(namespace, name))
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
		slice, ok := obj.(*resourcev1beta1.ResourceSlice)
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
		if !strings.HasPrefix(status.Device, "usb") {
			continue
		}

		if _, exists := allocResultsByPool[status.Pool]; !exists {
			allocResultsByPool[status.Pool] = make(map[string]resourcev1beta1.DeviceRequestAllocationResult)
		}

		allocResultsByPool[status.Pool][status.Device] = status
	}

	var myDevices []resourcev1beta1.Device
	var otherDevices []resourcev1beta1.Device

	for pool, allocResultsByDevice := range allocResultsByPool {
		resourceSlices, ok := byPoolSlices[pool]
		if !ok {
			return nil, nil, fmt.Errorf("no resource slices found for pool %s", pool)
		}

		for _, slice := range resourceSlices {
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

func addFinalizer(obj metav1.Object) bool {
	var newFinalizers []string
	for _, fin := range obj.GetFinalizers() {
		if fin == finalizer {
			return false
		}
		newFinalizers = append(newFinalizers, fin)
	}
	newFinalizers = append(newFinalizers, finalizer)
	obj.SetFinalizers(newFinalizers)
	return true
}

func removeFinalizer(obj metav1.Object) bool {
	var newFinalizers []string
	for _, fin := range obj.GetFinalizers() {
		if fin == finalizer {
			continue
		}
		newFinalizers = append(newFinalizers, fin)
	}
	if len(newFinalizers) == len(obj.GetFinalizers()) {
		return false
	}

	obj.SetFinalizers(newFinalizers)
	return true
}

func makeAddFinalizerPatch(obj metav1.Object) ([]byte, error) {
	var newFinalizers []string
	for _, fin := range obj.GetFinalizers() {
		if fin == finalizer {
			return nil, nil
		}
		newFinalizers = append(newFinalizers, fin)
	}
	newFinalizers = append(newFinalizers, finalizer)

	return patch.NewJSONPatch(
		patch.WithTest("/metadata/finalizers", obj.GetFinalizers()),
		patch.WithReplace("/metadata/finalizers", newFinalizers),
	).Bytes()
}

func makeRemoveFinalizerPatch(obj metav1.Object) ([]byte, error) {
	var newFinalizers []string
	for _, fin := range obj.GetFinalizers() {
		if fin == finalizer {
			return nil, nil
		}
		newFinalizers = append(newFinalizers, fin)
	}
	newFinalizers = append(newFinalizers, finalizer)

	return patch.NewJSONPatch(
		patch.WithTest("/metadata/finalizers", obj.GetFinalizers()),
		patch.WithReplace("/metadata/finalizers", newFinalizers),
	).Bytes()
}
