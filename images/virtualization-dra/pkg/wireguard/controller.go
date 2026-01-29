package wireguard

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"k8s.io/apimachinery/pkg/api/equality"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	drav1alpha1 "github.com/deckhouse/virtualization-dra/api/client/generated/clientset/versioned/typed/api/v1alpha1"
	vdraapi "github.com/deckhouse/virtualization-dra/api/v1alpha1"
	"github.com/deckhouse/virtualization-dra/pkg/controller"
)

const controllerName = "wireguard-controller"

type KeyStore interface {
	SetMyKeys(ctx context.Context, privateKey, publicKey wgtypes.Key) error
	GetMyKeys(ctx context.Context) (*wgtypes.Key, *wgtypes.Key, error)
	GetPeerPublicKeys(ctx context.Context) ([]wgtypes.Key, error)
}
type Controller struct {
	queue                         workqueue.TypedRateLimitingInterface[string]
	wireguardSystemNetworkIndexer cache.Indexer
	vdraClient                    drav1alpha1.UsbgatewayV1alpha1Interface
	wireguardSystemNetworkNameKey string
	wireguardSystemNetworkName    string
	namespace                     string
	nodeName                      string
	podIP                         string
	wireguardManager              *wgManager
	routeManager                  *routeManager
	afterHook                     Hook
	log                           *slog.Logger
	hasSynced                     cache.InformerSynced
	ready                         atomic.Bool
}

type Hook func(ctx context.Context) error

func NewController(wireguardSystemNetworkName, nodeName, namespace, podIP string, rouTableID int, wireguardSystemNetworkInformer cache.SharedIndexInformer, vdraClient drav1alpha1.UsbgatewayV1alpha1Interface, afterHook Hook) (*Controller, error) {
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: controllerName},
	)

	c := &Controller{
		queue:                         queue,
		wireguardSystemNetworkIndexer: wireguardSystemNetworkInformer.GetIndexer(),
		vdraClient:                    vdraClient,
		wireguardSystemNetworkNameKey: controller.KeyFunc(namespace, wireguardSystemNetworkName),
		wireguardSystemNetworkName:    wireguardSystemNetworkName,
		namespace:                     namespace,
		nodeName:                      nodeName,
		podIP:                         podIP,
		wireguardManager:              &wgManager{},
		routeManager:                  newRouteManager(rouTableID),
		afterHook:                     afterHook,
		log:                           slog.With(slog.String("controller", controllerName)),
	}

	_, err := wireguardSystemNetworkInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addWSN,
		UpdateFunc: c.updateWSN,
		DeleteFunc: c.deleteWSN,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add event handler to secret informer: %w", err)
	}

	c.hasSynced = func() bool {
		return wireguardSystemNetworkInformer.HasSynced()
	}

	return c, nil
}

func (c *Controller) addWSN(obj interface{}) {
	if wsn, ok := obj.(*vdraapi.WireguardSystemNetwork); ok && c.isMyWSN(wsn) {
		c.enqueueWSN(wsn)
	}
}

func (c *Controller) updateWSN(oldObj, newObj interface{}) {
	oldWSN, oldOk := oldObj.(*vdraapi.WireguardSystemNetwork)
	newWSN, newOk := newObj.(*vdraapi.WireguardSystemNetwork)

	if oldOk && newOk && c.isMyWSN(newWSN) {
		if equality.Semantic.DeepEqual(oldWSN.Status, newWSN.Status) {
			c.enqueueWSN(newWSN)
		}
	}
}

func (c *Controller) deleteWSN(obj interface{}) {
	if wsn, ok := obj.(*vdraapi.WireguardSystemNetwork); ok && c.isMyWSN(wsn) {
		c.enqueueWSN(wsn)
	}
}

func (c *Controller) enqueueWSN(wsn *vdraapi.WireguardSystemNetwork) {
	key, err := controller.ObjectKeyFunc(wsn)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %w", wsn, err))
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
	log := c.log.With(slog.String("key", key))
	log.Info("syncing wireguard system network")

	if !c.isMyWSNKey(key) {
		log.Warn("False trying to sync other wireguard system network")
		return nil
	}

	wsn, err := c.getWireguardSystemNetwork()
	if err != nil {
		return err
	}

	if wsn == nil {
		log.Info("Wireguard system network not found; it may have been deleted.")
		return nil
	}

	if !wsn.DeletionTimestamp.IsZero() {
		log.Info("Wireguard system network is being deleted; skip...")
		return nil
	}

	if err = c.updateNodeSettingsIfNeeded(ctx, wsn); err != nil {
		return fmt.Errorf("failed to update node settings: %w", err)
	}

	privateKey, publicKey, endpoint := c.getMyKeysAndEndpoint(wsn)
	if privateKey == nil || publicKey == nil || endpoint == "" {
		return fmt.Errorf("failed to find nodeSettings after update, please report a bug, this should not happen")
	}

	if err = c.configureWireguard(ctx, wsn); err != nil {
		return err
	}

	if c.afterHook != nil {
		if err = c.afterHook(ctx); err != nil {
			return fmt.Errorf("failed to run after hook: %w", err)
		}
	}

	c.ready.Store(true)

	return nil
}

func (c *Controller) isMyWSN(wsn *vdraapi.WireguardSystemNetwork) bool {
	return wsn.Namespace == c.namespace && wsn.Name == c.wireguardSystemNetworkName
}

func (c *Controller) isMyWSNKey(key string) bool {
	return controller.KeyFunc(c.namespace, c.wireguardSystemNetworkName) == key
}

func (c *Controller) configureWireguard(_ context.Context, wsn *vdraapi.WireguardSystemNetwork) error {
	priv, pub, _ := c.getMyKeysAndEndpoint(wsn)
	if priv == nil || pub == nil {
		return fmt.Errorf("failed to find nodeSettings after update, please report a bug, this should not happen")
	}

	peers, err := c.getPeers(wsn)
	if err != nil {
		return fmt.Errorf("failed to get peers: %w", err)
	}

	config := NewConfig(wsn.Spec.ListenPort, *priv, *pub, peers)

	allocAddr := c.getAllocatedAddr(wsn)
	if allocAddr == nil {
		return fmt.Errorf("not found allocated address: %w", err)
	}

	err = c.wireguardManager.ConfigureDevice(wsn.Spec.InterfaceName, allocAddr, config)
	if err != nil {
		return fmt.Errorf("failed to configure WireguardSystemNetwork: %w", err)
	}

	var cidrs []*net.IPNet
	for _, alloc := range wsn.Status.AllocatedIPs {
		if alloc.Node != c.nodeName {
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", alloc.IP, 32))
			if err != nil {
				c.log.Warn("failed to parse CIDR from allocated IP", slog.String("ip", alloc.IP), slog.Any("error", err))
				continue
			}
			cidrs = append(cidrs, cidr)
		}
	}
	err = c.routeManager.Sync(wsn.Spec.InterfaceName, cidrs)

	return nil
}

func (c *Controller) getAllocatedAddr(wsn *vdraapi.WireguardSystemNetwork) *net.IPNet {
	for _, alloc := range wsn.Status.AllocatedIPs {
		if alloc.Node == c.nodeName {
			ipNet := net.IPNet{
				IP:   net.ParseIP(alloc.IP),
				Mask: net.CIDRMask(32, 32),
			}
			return &ipNet
		}
	}
	return nil
}

func (c *Controller) getWireguardSystemNetwork() (*vdraapi.WireguardSystemNetwork, error) {
	obj, exists, err := c.wireguardSystemNetworkIndexer.GetByKey(c.wireguardSystemNetworkNameKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get WireguardSystemNetwork: %w", err)
	}
	if !exists {
		return nil, nil
	}
	wsn, ok := obj.(*vdraapi.WireguardSystemNetwork)
	if !ok {
		return nil, fmt.Errorf("unexpected type of WireguardSystemNetwork: %T", obj)
	}
	return wsn.DeepCopy(), nil
}
