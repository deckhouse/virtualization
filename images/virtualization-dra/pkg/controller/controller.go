package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func ObjectKeyFunc(obj interface{}) (string, error) {
	return cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
}

func KeyFunc(namespace, name string) string {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}.String()
}

type Controller interface {
	Queue() workqueue.TypedRateLimitingInterface[string]
	HasSynced() bool
	Sync(ctx context.Context, key string) error
	Logger() *slog.Logger
}

func Run(controller Controller, ctx context.Context, workers int) error {
	return newController(controller).Run(ctx, workers)
}

func newController(c Controller) *controller {
	return &controller{
		controller: c,
		queue:      c.Queue(),
		log:        c.Logger(),
	}
}

type controller struct {
	controller Controller
	queue      workqueue.TypedRateLimitingInterface[string]
	log        *slog.Logger
}

func (c *controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.log.Info("Starting controller")
	defer c.log.Info("Shutting down controller")

	if !cache.WaitForCacheSync(ctx.Done(), c.controller.HasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.log.Info("Starting workers controller")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.worker, time.Second)
	}

	<-ctx.Done()
	return nil
}

func (c *controller) worker(ctx context.Context) {
	workFunc := func(ctx context.Context) bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.controller.Sync(ctx, key); err != nil {
			c.log.Error("re-enqueuing", slog.String("key", key), slog.Any("err", err))
			c.queue.AddRateLimited(key)
		} else {
			c.log.Info(fmt.Sprintf("processed %v", key))
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
