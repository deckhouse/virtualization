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

package observer

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

const deletedPollInterval = time.Second

// IsDeleted reports whether the resource identified by (name, namespace) no
// longer exists. When provided to WaitForDeleted, it is polled alongside the
// watch so fast deletions that happen before the watch starts are not missed.
type IsDeleted func(ctx context.Context) (bool, error)

// WaitForDeleted blocks until a watch.Deleted event is observed for the
// resource identified by (name, namespace), or isDeleted reports that it is
// already gone.
func WaitForDeleted(
	ctx context.Context,
	w Watcher,
	name, namespace string,
	timeout time.Duration,
	isDeleted IsDeleted,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if isDeleted != nil {
		gone, err := isDeleted(ctx)
		if err != nil {
			return fmt.Errorf("observer: check deletion of %s/%s: %w", namespace, name, err)
		}
		if gone {
			return nil
		}
	}

	wi, err := w.Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
	if err != nil {
		return fmt.Errorf("observer: start watch for deletion of %s/%s: %w", namespace, name, err)
	}
	defer wi.Stop()

	poll := time.NewTicker(deletedPollInterval)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			if isDeleted != nil {
				gone, err := isDeleted(ctx)
				if err != nil {
					return fmt.Errorf("observer: check deletion of %s/%s: %w", namespace, name, err)
				}
				if gone {
					return nil
				}
			}
			return fmt.Errorf("observer: wait for deletion of %s/%s timed out after %s: %w", namespace, name, timeout, ctx.Err())
		case <-poll.C:
			if isDeleted == nil {
				continue
			}
			gone, err := isDeleted(ctx)
			if err != nil {
				return fmt.Errorf("observer: check deletion of %s/%s: %w", namespace, name, err)
			}
			if gone {
				return nil
			}
		case event, ok := <-wi.ResultChan():
			if !ok {
				if isDeleted != nil {
					gone, err := isDeleted(ctx)
					if err != nil {
						return fmt.Errorf("observer: check deletion of %s/%s: %w", namespace, name, err)
					}
					if gone {
						return nil
					}
				}
				return fmt.Errorf("observer: watch closed before %s/%s was deleted", namespace, name)
			}
			if event.Type != watch.Deleted {
				continue
			}
			obj, ok := event.Object.(metav1.Object)
			if !ok {
				continue
			}
			if obj.GetName() == name && obj.GetNamespace() == namespace {
				return nil
			}
		}
	}
}

// DynamicWatcher returns a Watcher for a dynamic API resource.
func DynamicWatcher(client dynamic.Interface, gvr schema.GroupVersionResource, namespace string) Watcher {
	return &dynamicResourceWatcher{
		client:    client,
		gvr:       gvr,
		namespace: namespace,
	}
}

type dynamicResourceWatcher struct {
	client    dynamic.Interface
	gvr       schema.GroupVersionResource
	namespace string
}

func (d *dynamicResourceWatcher) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	if d.namespace != "" {
		return d.client.Resource(d.gvr).Namespace(d.namespace).Watch(ctx, opts)
	}
	return d.client.Resource(d.gvr).Watch(ctx, opts)
}
