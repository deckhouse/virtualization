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

package storageprofile

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
)

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
			converted, err := convertStorageProfileEvent(event)
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

func convertStorageProfileEvent(event watch.Event) (watch.Event, error) {
	u, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return event, nil
	}

	sp, err := unstructuredToStorageProfile(u)
	if err != nil {
		return watch.Event{}, err
	}

	return watch.Event{Type: event.Type, Object: sp}, nil
}

func unstructuredToStorageProfile(u *unstructured.Unstructured) (*storagev1alpha1.StorageProfile, error) {
	raw, err := u.MarshalJSON()
	if err != nil {
		return nil, err
	}

	sp := &storagev1alpha1.StorageProfile{}
	if err := json.Unmarshal(raw, sp); err != nil {
		return nil, err
	}
	return sp, nil
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
