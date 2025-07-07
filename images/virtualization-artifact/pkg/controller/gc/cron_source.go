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

package gc

import (
	"cmp"
	"context"
	"fmt"
	"slices"
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

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

var _ source.Source = &CronSource{}

const sourceName = "CronSource"

func NewCronSource(c client.Client,
	standardSpec string,
	objList client.ObjectList,
	option CronSourceOption,
	log *log.Logger,
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
	log          *log.Logger
}

type CronSourceOption struct {
	GetOlder func(objList client.ObjectList) client.ObjectList
}

func NewDefaultCronSourceOption(objs client.ObjectList, ttl time.Duration, log *log.Logger) CronSourceOption {
	return CronSourceOption{
		GetOlder: DefaultGetOlder(objs, ttl, 10, log),
	}
}

func DefaultGetOlder(objs client.ObjectList, ttl time.Duration, maxCount int, log *log.Logger) func(objList client.ObjectList) client.ObjectList {
	return func(objList client.ObjectList) client.ObjectList {
		var expiredItems []runtime.Object
		var notExpiredItems []runtime.Object

		if err := meta.EachListItem(objList, func(o runtime.Object) error {
			obj, ok := o.(client.Object)
			if !ok {
				return nil
			}
			if object.GetAge(obj) > ttl {
				expiredItems = append(expiredItems, o)
			} else {
				notExpiredItems = append(notExpiredItems, o)
			}

			return nil
		}); err != nil {
			log.Error("failed to populate list", logger.SlogErr(err))
		}

		if maxCount != 0 && len(notExpiredItems) > maxCount {
			slices.SortFunc(notExpiredItems, func(a, b runtime.Object) int {
				aObj, _ := a.(client.Object)
				bObj, _ := b.(client.Object)

				return cmp.Compare(object.GetAge(aObj), object.GetAge(bObj))
			})
			expiredItems = append(expiredItems, notExpiredItems[maxCount:]...)
		}

		if err := meta.SetList(objs, expiredItems); err != nil {
			log.Error("failed to set list", logger.SlogErr(err))
		}
		return objs
	}
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
