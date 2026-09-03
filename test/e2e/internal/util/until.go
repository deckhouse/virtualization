/*
Copyright 2025 Flant JSC

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

package util

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// UntilObjectPhase waits for an object to reach the specified phase.
// It accepts a runtime.Object (which serves as a template with name and namespace),
// expected phase string, and timeout duration.
// The GVK is automatically extracted from the object via the client's scheme.
func UntilObjectPhase(ctx context.Context, expectedPhase string, timeout time.Duration, objs ...client.Object) {
	GinkgoHelper()
	untilObjectField(ctx, "status.phase", expectedPhase, timeout, objs...)
}

// UntilObjectsDeleted waits for objects to be deleted, watching each object
// through a dynamic watch (with a polling fallback for deletions that happen
// before the watch starts).
func UntilObjectsDeleted(ctx context.Context, timeout time.Duration, objs ...client.Object) {
	GinkgoHelper()

	for _, obj := range objs {
		u := getTemplateUnstructured(obj)
		watcher := dynamicWatcherFor(u.GroupVersionKind(), obj.GetNamespace())

		err := observer.WaitForDeleted(ctx, watcher, obj.GetName(), obj.GetNamespace(), timeout,
			func(ctx context.Context) (bool, error) {
				current := u.DeepCopy()
				getErr := framework.GetClients().GenericClient().Get(ctx, client.ObjectKeyFromObject(obj), current)
				if k8serrors.IsNotFound(getErr) {
					return true, nil
				}
				return false, getErr
			})
		Expect(err).NotTo(HaveOccurred(), "object %s must be removed", extractObjectNamespacedNameString(obj))
	}
}

// extractField extracts a string value from an unstructured object at the provided fieldPath (dot-separated, e.g. "status.phase" or "metadata.name").
func extractField(obj client.Object, fieldPath string) string {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return "Unknown"
	}
	path := strings.Split(fieldPath, ".")
	value, found, err := unstructured.NestedString(u.Object, path...)
	if err != nil || !found {
		return "Unknown"
	}
	return value
}

// untilObjectField waits for an object field to reach the specified value.
// It accepts a runtime.Object (which serves as a template with name and namespace),
// fieldPath (dot-separated path to the field, e.g. "status.phase" or "metadata.name"),
// expected value string, field name for error messages, and timeout duration.
// The GVK is automatically extracted from the object via the client's scheme.
// Each object is observed through a dynamic watch.
func untilObjectField(ctx context.Context, fieldPath, expectedValue string, timeout time.Duration, objs ...client.Object) {
	GinkgoHelper()

	for _, obj := range objs {
		u := getTemplateUnstructured(obj)

		obs, err := observer.New[*unstructured.Unstructured](
			ctx,
			dynamicWatcherFor(u.GroupVersionKind(), obj.GetNamespace()),
			obj.GetName(), obj.GetNamespace(),
		)
		Expect(err).NotTo(HaveOccurred(), "failed to start observer for object %s", extractObjectNamespacedNameString(obj))

		// The watch delivers the current state as its first (synthetic Added)
		// event, but evaluate it explicitly as well so an object that settled
		// long ago cannot stall the wait if that event is missed.
		current := u.DeepCopy()
		if getErr := framework.GetClients().GenericClient().Get(ctx, client.ObjectKeyFromObject(obj), current); getErr == nil {
			if ok, _ := fieldEquals(fieldPath, expectedValue)(current); ok {
				obs.Stop()
				continue
			}
		}

		waitErr := obs.WaitFor(fieldEquals(fieldPath, expectedValue), timeout)
		obs.Stop()
		Expect(waitErr).NotTo(HaveOccurred(),
			"object %s %s did not become %s", extractObjectNamespacedNameString(obj), fieldPath, expectedValue)
	}
}

// dynamicWatcherFor resolves the GVK to its REST mapping and returns a
// dynamic-client watcher for the resource, cluster-scoped when the mapping
// says so.
func dynamicWatcherFor(gvk schema.GroupVersionKind, namespace string) observer.Watcher {
	GinkgoHelper()

	mapping, err := framework.GetClients().GenericClient().RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	Expect(err).NotTo(HaveOccurred(), "failed to resolve REST mapping for %s", gvk)

	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		namespace = ""
	}

	return observer.DynamicWatcher(framework.GetClients().DynamicClient(), mapping.Resource, namespace)
}

func getTemplateUnstructured(obj client.Object) *unstructured.Unstructured {
	// Convert the template object to unstructured once
	var templateUnstructured *unstructured.Unstructured
	var gvk schema.GroupVersionKind

	// Handle two possible input formats:
	// 1. If the object is already unstructured, use it directly with its GVK
	// 2. If it's a typed struct (e.g., VirtualMachine), convert it to unstructured
	//    and extract GVK from the client's scheme registry
	if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
		// Object is already unstructured - just copy it and extract GVK
		templateUnstructured = unstructuredObj.DeepCopy()
	} else {
		// Object is a typed struct - convert to unstructured format
		objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		Expect(err).NotTo(HaveOccurred(), "failed to convert object to unstructured")
		templateUnstructured = &unstructured.Unstructured{Object: objMap}

		// Get GVK from the scheme (which knows about registered types)
		client := framework.GetClients().GenericClient()
		gvks, _, err := client.Scheme().ObjectKinds(obj)
		Expect(err).NotTo(HaveOccurred(), "failed to get GVK from object")
		Expect(len(gvks)).To(BeNumerically(">", 0), "no GVK found for object")
		gvk = gvks[0]
		templateUnstructured.SetGroupVersionKind(gvk)
	}
	return templateUnstructured
}

func extractObjectNamespacedNameString(obj client.Object) string {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	divider := ""
	if namespace != "" {
		divider = "/"
	}

	return fmt.Sprintf("%s%s%s", namespace, divider, name)
}

// fieldEquals reports the object's dot-separated string field equals the
// expected value.
func fieldEquals(fieldPath, expectedValue string) observer.Predicate[*unstructured.Unstructured] {
	return func(u *unstructured.Unstructured) (bool, error) {
		return extractField(u, fieldPath) == expectedValue, nil
	}
}
