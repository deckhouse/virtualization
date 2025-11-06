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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// UntilObjectPhase waits for an object to reach the specified phase.
// It accepts a runtime.Object (which serves as a template with name and namespace),
// expected phase string, and timeout duration.
// The GVK is automatically extracted from the object via the client's scheme.
func UntilObjectPhase(expectedPhase string, timeout time.Duration, objs ...runtime.Object) {
	GinkgoHelper()
	untilObjectField("status.phase", expectedPhase, timeout, objs...)
}

// UntilObjectState waits for an object to reach the specified state.
// It accepts a runtime.Object (which serves as a template with name and namespace),
// expected state string, and timeout duration.
// The GVK is automatically extracted from the object via the client's scheme.
func UntilObjectState(expectedState string, timeout time.Duration, objs ...runtime.Object) {
	GinkgoHelper()
	untilObjectField("status.state", expectedState, timeout, objs...)
}

// extractField extracts a string value from an unstructured object at the provided fieldPath (dot-separated, e.g. "status.phase" or "metadata.name").
func extractField(obj client.Object, fieldPath string) string {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return "Unknown"
	}
	path := make([]string, 0)
	for _, part := range splitFieldPath(fieldPath) {
		if part != "" {
			path = append(path, part)
		}
	}
	value, found, err := unstructured.NestedString(u.Object, path...)
	if err != nil || !found {
		return "Unknown"
	}
	return value
}

// splitFieldPath splits a dot-separated field path into its parts.
func splitFieldPath(fieldPath string) []string {
	var parts []string
	curr := ""
	for i := 0; i < len(fieldPath); i++ {
		if fieldPath[i] == '.' {
			parts = append(parts, curr)
			curr = ""
		} else {
			curr += string(fieldPath[i])
		}
	}
	if curr != "" {
		parts = append(parts, curr)
	}
	return parts
}

// untilObjectField waits for an object field to reach the specified value.
// It accepts a runtime.Object (which serves as a template with name and namespace),
// fieldPath (dot-separated path to the field, e.g. "status.phase" or "metadata.name"),
// expected value string, field name for error messages, and timeout duration.
// The GVK is automatically extracted from the object via the client's scheme.
func untilObjectField(fieldPath, expectedValue string, timeout time.Duration, objs ...runtime.Object) {
	Eventually(func(g Gomega) {
		for _, obj := range objs {
			// Get name and namespace from client.Object
			clientObj, ok := obj.(client.Object)
			Expect(ok).To(BeTrue(), "object must implement client.Object interface")
			key := client.ObjectKeyFromObject(clientObj)
			name := clientObj.GetName()
			namespace := clientObj.GetNamespace()

			// Create a new unstructured object for each Get call
			u := getTemplateUnstructured(obj).DeepCopy()
			err := framework.GetClients().GenericClient().Get(context.Background(), key, u)
			if err != nil {
				g.Expect(err).NotTo(HaveOccurred(), "failed to get object %s/%s", namespace, name)
			}

			value := extractField(u, fieldPath)
			g.Expect(value).To(Equal(expectedValue), "object %s/%s %s is %s, expected %s", namespace, name, fieldPath, value, expectedValue)
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func getTemplateUnstructured(obj runtime.Object) *unstructured.Unstructured {
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
