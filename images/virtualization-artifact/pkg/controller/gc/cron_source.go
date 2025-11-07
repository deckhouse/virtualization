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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

/**
CronSource is an implementation of the controller-runtime Source interface.
It periodically triggers and emit events for a list of objects.

The component is independent of kubernetes client: developer should implement
ObjectLister interface that CronSource will use to determine what to enqueue on trigger.

NewSingleObjectLister can be used if the main objective is to get periodical event,
but specific namespace and name are not important. Also, for this situation
object name can be used to distinguish cron trigger from the kubernetes trigger.
*/

var _ source.Source = &CronSource{}

const sourceName = "CronSource"

func NewCronSource(scheduleSpec string, objLister ObjectLister, log *log.Logger) (*CronSource, error) {
	specParser := cron.NewParser(cron.Second | cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := specParser.Parse(scheduleSpec)
	if err != nil {
		return nil, fmt.Errorf("parse schedule: %v\n", err)
	}

	return &CronSource{
		schedule:  schedule,
		objLister: objLister,
		log:       log.With("watchSource", sourceName),
		clock:     &clock.RealClock{},
	}, nil
}

type CronSource struct {
	schedule  cron.Schedule
	objLister ObjectLister
	log       *log.Logger
	clock     clock.Clock
}

func (c *CronSource) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	nextTime := nextScheduleTimeDuration(c.schedule, c.clock.Now())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.clock.After(nextTime):
				c.enqueueObjects(ctx, queue.Add)
				nextTime = nextScheduleTimeDuration(c.schedule, c.clock.Now())
			}
		}
	}()
	return nil
}

func (c *CronSource) enqueueObjects(ctx context.Context, queueAddFunc func(reconcile.Request)) {
	now := c.clock.Now()
	objs, err := c.objLister.List(ctx, now)
	if err != nil {
		c.log.Error("Failed to get ObjectList for delete", logger.SlogErr(err))
		return
	}

	if len(objs) == 0 {
		c.log.Debug(fmt.Sprintf("No resources at %s, skip queueing", now))
		return
	}

	for _, obj := range objs {
		queueAddFunc(reconcile.Request{
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

type ObjectLister interface {
	List(ctx context.Context, now time.Time) ([]client.Object, error)
}

type ObjectListerImpl struct {
	ListFunc func(ctx context.Context, now time.Time) ([]client.Object, error)
}

func (o *ObjectListerImpl) List(ctx context.Context, now time.Time) ([]client.Object, error) {
	if o.ListFunc == nil {
		return nil, nil
	}
	return o.ListFunc(ctx, now)
}

func NewObjectLister(listFunc func(ctx context.Context, now time.Time) ([]client.Object, error)) *ObjectListerImpl {
	return &ObjectListerImpl{listFunc}
}

func NewSingleObjectLister(namespace, name string) *ObjectListerImpl {
	return &ObjectListerImpl{ListFunc: func(ctx context.Context, now time.Time) ([]client.Object, error) {
		return []client.Object{&metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
		}}, nil
	}}
}
