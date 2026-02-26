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

package usbgateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/deckhouse/virtualization-dra/pkg/controller"
	"github.com/deckhouse/virtualization-dra/pkg/patch"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

const controllerName = "usb-gateway-controller"

type USBGatewayController struct {
	secretName           string
	namespace            string
	nodeName             string
	usbipdAddr           string
	client               kubernetes.Interface
	secretIndexer        cache.Indexer
	resourceSliceIndexer cache.Indexer
	usbIP                usbip.Interface
	queue                workqueue.TypedRateLimitingInterface[string]
	hasSynced            cache.InformerSynced
	attachRecordManager  *attachRecordManager

	mu            sync.RWMutex
	nodeAddresses map[string]string

	log *slog.Logger
}

func NewUSBGatewayController(
	ctx context.Context,
	secretName, namespace, nodeName, usbipdHost string,
	usbipdPort string,
	client kubernetes.Interface,
	secretInformer, resourceSliceInformer cache.SharedIndexInformer,
	usbIP usbip.Interface,
) (*USBGatewayController, error) {
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: controllerName},
	)

	attachRecordManager, err := newAttachRecordManager(DefaultRecordStateDir, usbIP)
	if err != nil {
		return nil, err
	}

	c := &USBGatewayController{
		secretName:           secretName,
		namespace:            namespace,
		nodeName:             nodeName,
		usbipdAddr:           net.JoinHostPort(usbipdHost, usbipdPort),
		client:               client,
		secretIndexer:        secretInformer.GetIndexer(),
		resourceSliceIndexer: resourceSliceInformer.GetIndexer(),
		usbIP:                usbIP,
		queue:                queue,
		log:                  slog.With(slog.String("controller", controllerName)),
		attachRecordManager:  attachRecordManager,
	}

	_, err = secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addSecret,
		UpdateFunc: c.updateSecret,
		DeleteFunc: c.deleteSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add event handler to secret informer: %w", err)
	}

	c.hasSynced = func() bool {
		return secretInformer.HasSynced() && resourceSliceInformer.HasSynced()
	}

	err = c.runSecretChecker(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to run secret checker: %w", err)
	}

	return c, nil
}

func (c *USBGatewayController) runSecretChecker(ctx context.Context) error {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	if !cache.WaitForCacheSync(ctx.Done(), c.hasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	key := controller.KeyFunc(c.namespace, c.secretName)
	c.queueAdd(key)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				exist, err := c.secretExists(key)
				if err != nil {
					c.log.Error("Failed to check secret existence", slog.Any("error", err))
				}
				if !exist {
					c.queueAdd(key)
				}
			}
		}
	}()

	return nil
}

func (c *USBGatewayController) addSecret(obj interface{}) {
	if secret, ok := obj.(*corev1.Secret); ok && c.isMySecret(secret) {
		c.enqueueSecret(secret)
	} else if !ok {
		c.log.Error("expected secret, got", slog.Any("obj", obj))
	}
}

func (c *USBGatewayController) deleteSecret(obj interface{}) {
	if secret, ok := obj.(*corev1.Secret); ok && c.isMySecret(secret) {
		c.enqueueSecret(secret)
	} else if !ok {
		c.log.Error("expected secret, got", slog.Any("obj", obj))
	}
}

func (c *USBGatewayController) updateSecret(oldObj, newObj interface{}) {
	oldSecret, oldOk := oldObj.(*corev1.Secret)
	newSecret, newOk := newObj.(*corev1.Secret)

	if !oldOk || !newOk {
		c.log.Error("expected secret, got", slog.Any("old", oldObj), slog.Any("new", newObj))
		return
	}

	if c.isMySecret(newSecret) && !equality.Semantic.DeepEqual(oldSecret.Data, newSecret.Data) {
		c.enqueueSecret(newSecret)
	}
}

func (c *USBGatewayController) isMySecret(secret *corev1.Secret) bool {
	return secret.Name == c.secretName && secret.Namespace == c.namespace
}

func (c *USBGatewayController) isMySecretKey(key string) bool {
	return key == controller.KeyFunc(c.namespace, c.secretName)
}

func (c *USBGatewayController) enqueueSecret(secret *corev1.Secret) {
	c.queueAdd(controller.MetaObjectKeyFunc(secret))
}

func (c *USBGatewayController) queueAdd(key string) {
	c.queue.Add(key)
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
	log.Info("syncing resource claim")

	if !c.isMySecretKey(key) {
		log.Error("False try reconcile other secret, please report a bug")
		return nil
	}

	secret, err := c.getSecret(key)
	if err != nil {
		return err
	}
	if secret == nil {
		return c.createSecret(ctx)
	}

	secret, err = c.ensureAddress(ctx, secret)
	if err != nil {
		return err
	}

	c.syncAddresses(secret)

	return nil
}

func (c *USBGatewayController) secretExists(key string) (bool, error) {
	_, exists, err := c.secretIndexer.GetByKey(key)
	return exists, err
}

func (c *USBGatewayController) getSecret(key string) (*corev1.Secret, error) {
	obj, exists, err := c.secretIndexer.GetByKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}
	if !exists {
		return nil, nil
	}
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil, fmt.Errorf("expected secret, got %T", obj)
	}
	return secret.DeepCopy(), nil
}

func (c *USBGatewayController) createSecret(ctx context.Context) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.secretName,
			Namespace: c.namespace,
		},
		Data: map[string][]byte{
			c.nodeName: []byte(c.usbipdAddr),
		},
	}

	_, err := c.client.CoreV1().Secrets(c.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func (c *USBGatewayController) ensureAddress(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	addr, exists := secret.Data[c.nodeName]
	if string(addr) == c.usbipdAddr {
		return secret, nil
	}

	jp := patch.NewJSONPatch(patch.WithTest("/data/"+c.nodeName, addr))
	if exists {
		jp.Append(patch.WithReplace("/data/"+c.nodeName, []byte(c.usbipdAddr)))
	} else {
		jp.Append(patch.WithAdd("/data/"+c.nodeName, []byte(c.usbipdAddr)))
	}

	bytes, err := jp.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to generate patch: %w", err)
	}

	newSecret, err := c.client.CoreV1().Secrets(c.namespace).Patch(ctx, secret.Name, types.JSONPatchType, bytes, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to patch secret: %w", err)
	}

	return newSecret, nil
}

func (c *USBGatewayController) syncAddresses(secret *corev1.Secret) {
	newAddresses := make(map[string]string, len(secret.Data))
	for node, addr := range secret.Data {
		newAddresses[node] = string(addr)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeAddresses = newAddresses
}
