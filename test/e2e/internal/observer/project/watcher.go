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

package project

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"

	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
)

// projectGVR is the cluster-scoped deckhouse.io Project resource. Project has no
// typed client in VirtClient, so the observer watches it through the dynamic
// client and decodes the unstructured events into dv1alpha2.Project.
var projectGVR = schema.GroupVersionResource{
	Group:    "deckhouse.io",
	Version:  "v1alpha2",
	Resource: "projects",
}

type dynamicWatcher struct {
	client dynamic.Interface
	gvr    schema.GroupVersionResource
}

func (w *dynamicWatcher) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	inner, err := w.client.Resource(w.gvr).Watch(ctx, opts)
	if err != nil {
		return nil, err
	}
	return newConvertingWatch(inner), nil
}

type convertingWatch struct {
	inner  watch.Interface
	events chan watch.Event
	stop   chan struct{}
}

func newConvertingWatch(inner watch.Interface) watch.Interface {
	cw := &convertingWatch{
		inner:  inner,
		events: make(chan watch.Event, 256),
		stop:   make(chan struct{}),
	}
	go cw.run()
	return cw
}

func (cw *convertingWatch) run() {
	defer close(cw.events)
	for {
		select {
		case <-cw.stop:
			return
		case event, ok := <-cw.inner.ResultChan():
			if !ok {
				return
			}
			converted, err := convertProjectEvent(event)
			if err != nil {
				continue
			}
			select {
			case <-cw.stop:
				return
			case cw.events <- converted:
			}
		}
	}
}

func convertProjectEvent(event watch.Event) (watch.Event, error) {
	u, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return event, nil
	}

	project, err := unstructuredToProject(u)
	if err != nil {
		return watch.Event{}, err
	}

	return watch.Event{Type: event.Type, Object: project}, nil
}

func unstructuredToProject(u *unstructured.Unstructured) (*dv1alpha2.Project, error) {
	raw, err := u.MarshalJSON()
	if err != nil {
		return nil, err
	}

	project := &dv1alpha2.Project{}
	if err := json.Unmarshal(raw, project); err != nil {
		return nil, err
	}
	return project, nil
}

func (cw *convertingWatch) Stop() {
	select {
	case <-cw.stop:
	default:
		close(cw.stop)
	}
	cw.inner.Stop()
}

func (cw *convertingWatch) ResultChan() <-chan watch.Event {
	return cw.events
}
