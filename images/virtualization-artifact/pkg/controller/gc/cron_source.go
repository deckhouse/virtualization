package gc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

var _ source.Source = &CronSource{}

const sourceName = "CronSource"

func NewCronSource(c client.Client,
	standardSpec string,
	objList client.ObjectList,
	option CronSourceOption,
	log *slog.Logger,
) *CronSource {
	return &CronSource{
		Client:       c,
		standardSpec: standardSpec,
		objList:      objList,
		option:       option,
		log:          log.With("WatchSource", sourceName),
	}
}

type CronSource struct {
	client.Client
	standardSpec string
	objList      client.ObjectList
	option       CronSourceOption
	log          *slog.Logger
}

type CronSourceOption struct {
	GetOlder func(objList client.ObjectList) client.ObjectList
}

func (c *CronSource) Start(ctx context.Context, _ handler.EventHandler, queue workqueue.RateLimitingInterface, predicates ...predicate.Predicate) error {
	schedule, err := cron.ParseStandard(c.standardSpec)
	if err != nil {
		return fmt.Errorf("parsing standard spec %q: %w", c.standardSpec, err)
	}
	work := func() {
		if err = meta.SetList(c.objList, nil); err != nil {
			c.log.Error("failed to reset resource list", logger.SlogErr(err))
			return
		}
		if err = c.List(ctx, c.objList); err != nil {
			c.log.Error("failed to listing resources", logger.SlogErr(err))
			return
		}
		if meta.LenList(c.objList) == 0 {
			c.log.Debug("no resources, skip")
			return
		}
		if c.option.GetOlder != nil {
			c.objList = c.option.GetOlder(c.objList)
		}
		if err = meta.EachListItem(c.objList, func(object runtime.Object) error {
			obj, ok := object.(client.Object)
			if !ok {
				c.log.Error(fmt.Sprintf("%s's type isn't metav1.Object", object.GetObjectKind().GroupVersionKind().String()))
				return nil
			}
			genericEvent := event.GenericEvent{Object: obj}
			for _, p := range predicates {
				if !p.Generic(genericEvent) {
					c.log.Debug(fmt.Sprintf("skip enqueue object %s/%s due to the predicate.", obj.GetNamespace(), obj.GetName()))
					return nil
				}
			}
			queue.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				},
			})
			c.log.Debug(fmt.Sprintf("resource %s/%s enqueued", obj.GetNamespace(), obj.GetName()))
			return nil
		}); err != nil {
			c.log.Error("failed to enqueueing resources", logger.SlogErr(err))
			return
		}
	}
	ta := nextScheduleTimeDuration(schedule, time.Now())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(ta):
				work()
				ta = nextScheduleTimeDuration(schedule, time.Now())
			}
		}
	}()
	return nil
}

func nextScheduleTimeDuration(schedule cron.Schedule, now time.Time) time.Duration {
	return schedule.Next(now).Sub(now)
}
