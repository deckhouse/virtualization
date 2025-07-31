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
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

var _ source.Source = &CronSource{}

const sourceName = "CronSource"

type SourceGCManager interface {
	ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error)
}

func NewCronSource(scheduleSpec string, client client.Client, mgr SourceGCManager, log *log.Logger) (*CronSource, error) {
	schedule, err := cron.ParseStandard(scheduleSpec)
	if err != nil {
		return nil, fmt.Errorf("parsing standard spec %q: %w", scheduleSpec, err)
	}

	return &CronSource{
		schedule: schedule,
		client:   client,
		mgr:      mgr,
		log:      log.With("WatchSource", sourceName),
		clock:    &clock.RealClock{},
	}, nil
}

type CronSource struct {
	schedule cron.Schedule
	client   client.Client
	mgr      SourceGCManager
	log      *log.Logger
	clock    clock.Clock
}

func (c *CronSource) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	nextTime := nextScheduleTimeDuration(c.schedule, c.clock.Now())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.clock.After(nextTime):
				c.addObjects(ctx, queue.Add)
				nextTime = nextScheduleTimeDuration(c.schedule, c.clock.Now())
			}
		}
	}()
	return nil
}

func (c *CronSource) addObjects(ctx context.Context, addToQueue func(reconcile.Request)) {
	objs, err := c.mgr.ListForDelete(ctx, c.clock.Now())
	if err != nil {
		c.log.Error("Failed to get ObjectList for delete", logger.SlogErr(err))
		return
	}

	if len(objs) == 0 {
		c.log.Debug("No resources, skip")
		return
	}

	for _, obj := range objs {
		addToQueue(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			},
		})
		c.log.Debug(fmt.Sprintf("Resource %s/%s enqueued", obj.GetNamespace(), obj.GetName()))
	}
}

func nextScheduleTimeDuration(schedule cron.Schedule, now time.Time) time.Duration {
	return schedule.Next(now).Sub(now)
}
