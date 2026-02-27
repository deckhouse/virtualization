package usbgateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/deckhouse/virtualization-dra/internal/consts"
	"github.com/deckhouse/virtualization-dra/pkg/controller"
	"github.com/deckhouse/virtualization-dra/pkg/patch"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

const controllerName = "usb-gateway-controller"

type USBGatewayController struct {
	nodeName             string
	usbipdAddr           string
	client               kubernetes.Interface
	nodeIndexer          cache.Indexer
	resourceSliceIndexer cache.Indexer
	usbIP                usbip.Interface
	marker               *Marker
	queue                workqueue.TypedRateLimitingInterface[string]
	hasSynced            cache.InformerSynced
	attachRecordManager  *attachRecordManager
	attachInfo           atomic.Value

	mu            sync.RWMutex
	nodeAddresses map[string]string

	log *slog.Logger
}

func NewUSBGatewayController(
	nodeName, usbipdHost, usbipdPort string,
	client kubernetes.Interface,
	dynamicClient dynamic.Interface,
	nodeInformer, resourceSliceInformer cache.SharedIndexInformer,
	usbIP usbip.Interface,
) (*USBGatewayController, error) {

	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: controllerName},
	)

	usbipAddr := net.JoinHostPort(usbipdHost, usbipdPort)
	if err := validateUSBIPAddress(usbipAddr); err != nil {
		return nil, fmt.Errorf("invalid usbip address: %w", err)
	}

	attachRecordManager, err := newAttachRecordManager(DefaultRecordStateDir, usbIP)
	if err != nil {
		return nil, err
	}

	c := &USBGatewayController{
		nodeName:             nodeName,
		usbipdAddr:           usbipAddr,
		client:               client,
		nodeIndexer:          nodeInformer.GetIndexer(),
		resourceSliceIndexer: resourceSliceInformer.GetIndexer(),
		usbIP:                usbIP,
		marker:               NewMarker(dynamicClient, nodeName),
		queue:                queue,
		log:                  slog.With(slog.String("controller", controllerName)),
		attachRecordManager:  attachRecordManager,
	}

	_, err = nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addNode,
		UpdateFunc: c.updateNode,
		DeleteFunc: c.deleteNode,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add event handler to node informer: %w", err)
	}

	c.hasSynced = func() bool {
		return nodeInformer.HasSynced() && resourceSliceInformer.HasSynced()
	}

	return c, nil
}

func (c *USBGatewayController) addNode(obj interface{}) {
	if node, ok := obj.(*corev1.Node); ok && c.nodeName == node.Name {
		c.enqueueNode(node)
	} else if !ok {
		c.log.Error("expected node, got", slog.Any("obj", obj))
	}
}

func (c *USBGatewayController) deleteNode(obj interface{}) {
	if node, ok := obj.(*corev1.Node); ok && c.nodeName == node.Name {
		c.enqueueNode(node)
	} else if !ok {
		c.log.Error("expected node, got", slog.Any("obj", obj))
	}
}

func (c *USBGatewayController) updateNode(oldObj, newObj interface{}) {
	_, oldOk := oldObj.(*corev1.Node)
	newNode, newOk := newObj.(*corev1.Node)

	if !oldOk || !newOk {
		c.log.Error("expected node, got", slog.Any("old", oldObj), slog.Any("new", newObj))
	}

	c.enqueueNode(newNode)
}

func (c *USBGatewayController) enqueueNode(node *corev1.Node) {
	c.queueAdd(controller.MetaObjectKeyFunc(node))
}

func (c *USBGatewayController) queueAdd(key string) {
	c.queue.Add(key)
}

func (c *USBGatewayController) queueAddAfter(key string, after time.Duration) {
	c.queue.AddAfter(key, after)
}

func (c *USBGatewayController) subscribeVhci(ctx context.Context) error {
	ch, err := c.usbIP.WatchAttachInfo(ctx)
	if err != nil {
		return err
	}

	go func() {
		for info := range ch {
			c.storeAttachInfo(info)
			c.queueAddAfter(c.nodeName, time.Second)
		}
	}()

	return nil
}

func (c *USBGatewayController) storeAttachInfo(info usbip.AttachInfo) {
	c.attachInfo.Store(info)
}

func (c *USBGatewayController) getAttachInfo() usbip.AttachInfo {
	return c.attachInfo.Load().(usbip.AttachInfo)
}

func (c *USBGatewayController) Start(ctx context.Context) error {
	if err := c.subscribeVhci(ctx); err != nil {
		return fmt.Errorf("failed to subscribe vhci: %w", err)
	}

	return nil
}

func (c *USBGatewayController) Stop() {
	if err := c.marker.Unmark(context.Background()); err != nil {
		c.log.Error("failed to unmark node", slog.Any("error", err))
	}
}

func (c *USBGatewayController) Queue() workqueue.TypedRateLimitingInterface[string] {
	return c.queue
}

func (c *USBGatewayController) HasSynced() bool {
	return c.hasSynced()
}

func (c *USBGatewayController) Logger() *slog.Logger {
	return c.log
}

func (c *USBGatewayController) Sync(ctx context.Context, key string) error {
	log := c.log.With("key", key)
	log.Info("syncing node")

	node, err := c.getNode(key)
	if err != nil {
		return err
	}

	c.syncAddresses(node, key)

	if node == nil || c.nodeName != node.Name {
		return nil
	}

	node, err = c.ensureAddress(ctx, node)
	if err != nil {
		return err
	}

	node, err = c.ensureAttachInfo(ctx, node)
	if err != nil {
		return err
	}

	return c.markNode(ctx)
}

func (c *USBGatewayController) getNode(key string) (*corev1.Node, error) {
	obj, exists, err := c.nodeIndexer.GetByKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", key, err)
	}
	if !exists {
		return nil, nil
	}
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil, fmt.Errorf("expected node, got %T", obj)
	}
	return node.DeepCopy(), nil
}

func (c *USBGatewayController) syncAddresses(node *corev1.Node, key string) {
	if node == nil {
		c.removeAddr(key)
		return
	}

	if node.Name == c.nodeName {
		c.addAddr(c.nodeName, c.usbipdAddr)
		return
	}

	addr, exists := node.Annotations[consts.AnnUSBIPAddress]
	if !exists {
		c.removeAddr(node.Name)
		return

	}

	err := validateUSBIPAddress(addr)
	if err != nil {
		c.log.Error("invalid address", slog.String("address", addr), slog.Any("error", err))
		c.removeAddr(node.Name)
		return
	}

	c.addAddr(node.Name, addr)
}

func (c *USBGatewayController) addAddr(node, addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.nodeAddresses == nil {
		c.nodeAddresses = make(map[string]string)
	}
	c.nodeAddresses[node] = addr
}

func (c *USBGatewayController) removeAddr(node string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.nodeAddresses, node)
}

func validateUSBIPAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("address is empty")
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid format (expected host:port): %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port number: %w", err)
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d out of range (1-65535)", port)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", host)
	}

	return nil
}

func (c *USBGatewayController) ensureAddress(ctx context.Context, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || c.nodeName != node.Name {
		return node, nil
	}

	addr, exists := node.Annotations[consts.AnnUSBIPAddress]
	if addr == c.usbipdAddr {
		return node, nil
	}

	path := "/metadata/annotations/" + patch.EscapeJSONPointer(consts.AnnUSBIPAddress)

	jp := patch.NewJSONPatch()
	if exists {
		jp.Append(patch.WithTest(path, addr), patch.WithReplace(path, c.usbipdAddr))
	} else {
		jp.Append(patch.WithAdd(path, c.usbipdAddr))
	}
	bytes, err := jp.Bytes()
	if err != nil {
		return node, fmt.Errorf("failed to generate patch: %w", err)
	}

	newNode, err := c.client.CoreV1().Nodes().Patch(ctx, node.Name, types.JSONPatchType, bytes, metav1.PatchOptions{})
	if err != nil {
		return node, fmt.Errorf("failed to patch node: %w", err)
	}

	return newNode, nil
}

func (c *USBGatewayController) ensureAttachInfo(ctx context.Context, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || c.nodeName != node.Name {
		return node, nil
	}

	attachInfo := c.getAttachInfo()

	nports := strconv.Itoa(attachInfo.NPorts)
	usedPorts := strconv.Itoa(len(attachInfo.Items))

	jp := patch.NewJSONPatch()

	nPortsPath := "/metadata/annotations/" + patch.EscapeJSONPointer(consts.AnnUSBIPTotalPorts)
	oldNPorts, exists := node.Annotations[consts.AnnUSBIPTotalPorts]

	if nports != oldNPorts {
		if exists {
			jp.Append(patch.WithTest(nPortsPath, oldNPorts), patch.WithReplace(nPortsPath, nports))
		} else {
			jp.Append(patch.WithAdd(nPortsPath, nports))
		}
	}

	usedPortsPath := "/metadata/annotations/" + patch.EscapeJSONPointer(consts.AnnUSBIPUsedPorts)
	oldUsedPorts, exists := node.Annotations[consts.AnnUSBIPUsedPorts]
	if usedPorts != oldUsedPorts {
		if exists {
			jp.Append(patch.WithTest(usedPortsPath, oldUsedPorts), patch.WithReplace(usedPortsPath, usedPorts))
		} else {
			jp.Append(patch.WithAdd(usedPortsPath, usedPorts))
		}
	}

	if jp.Len() == 0 {
		return node, nil
	}

	bytes, err := jp.Bytes()
	if err != nil {
		return node, fmt.Errorf("failed to generate patch: %w", err)
	}

	newNode, err := c.client.CoreV1().Nodes().Patch(ctx, node.Name, types.JSONPatchType, bytes, metav1.PatchOptions{})
	if err != nil {
		return node, fmt.Errorf("failed to patch node: %w", err)
	}

	return newNode, nil
}

func (c *USBGatewayController) markNode(ctx context.Context) error {
	attachInfo := c.getAttachInfo()

	if len(attachInfo.Items) >= attachInfo.NPorts {
		return c.marker.Unmark(ctx)
	}

	return c.marker.Mark(ctx)
}
