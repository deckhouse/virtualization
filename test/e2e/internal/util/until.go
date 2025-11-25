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
	"strings"
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
func UntilObjectPhase(expectedPhase string, timeout time.Duration, objs ...client.Object) {
	GinkgoHelper()
	untilObjectField("status.phase", expectedPhase, timeout, objs...)
}

// UntilConditionReason waits for the specified conditionType in status.conditions to have the given reason value for all provided objects.
func UntilConditionReason(conditionType, expectedReason string, timeout time.Duration, objs ...client.Object) {
	UntilConditionState(conditionType, timeout, struct {
		Reason       string
		Status       string
		Message      string
		CheckReason  bool
		CheckStatus  bool
		CheckMessage bool
	}{
		Reason:      expectedReason,
		CheckReason: true,
	}, objs...)
}

// UntilConditionStatus waits for the specified conditionType in status.conditions to have the given status value for all provided objects.
func UntilConditionStatus(conditionType, expectedStatus string, timeout time.Duration, objs ...client.Object) {
	UntilConditionState(conditionType, timeout, struct {
		Reason       string
		Status       string
		Message      string
		CheckReason  bool
		CheckStatus  bool
		CheckMessage bool
	}{
		Status:      expectedStatus,
		CheckStatus: true,
	}, objs...)
}

// UntilConditionState generalizes condition field checks ("reason", "status", "message") for the specified conditionType.
// You can specify which fields to check by setting the corresponding flags to true and providing their expected values.
func UntilConditionState(
	conditionType string,
	timeout time.Duration,
	checkOptions struct {
		Reason       string
		Status       string
		Message      string
		CheckReason  bool
		CheckStatus  bool
		CheckMessage bool
	},
	objs ...client.Object,
) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, obj := range objs {
			key := client.ObjectKeyFromObject(obj)
			u := getTemplateUnstructured(obj).DeepCopy()
			err := framework.GetClients().GenericClient().Get(context.Background(), key, u)
			g.Expect(err).ShouldNot(HaveOccurred())

			conditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
			g.Expect(err).ShouldNot(HaveOccurred(), "failed to access status.conditions of %s/%s", u.GetNamespace(), u.GetName())
			g.Expect(found).Should(BeTrue(), "no status.conditions found in %s/%s", u.GetNamespace(), u.GetName())

			var actualReason, actualStatus, actualMessage string
			condFound := false

			for _, c := range conditions {
				m, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				if t, ok := m["type"].(string); ok && t == conditionType {
					condFound = true
					if s, ok := m["reason"].(string); ok {
						actualReason = s
					} else {
						actualReason = "Unknown"
					}
					if s, ok := m["status"].(string); ok {
						actualStatus = s
					} else {
						actualStatus = "Unknown"
					}
					if s, ok := m["message"].(string); ok {
						actualMessage = s
					} else {
						actualMessage = ""
					}
					break
				}
			}
			g.Expect(condFound).To(BeTrue(), "object %s/%s: condition %s not found", u.GetNamespace(), u.GetName(), conditionType)

			if checkOptions.CheckReason {
				g.Expect(actualReason).To(Equal(checkOptions.Reason),
					"object %s/%s: condition %s reason is %q, expected %q",
					u.GetNamespace(), u.GetName(), conditionType, actualReason, checkOptions.Reason)
			}
			if checkOptions.CheckStatus {
				g.Expect(actualStatus).To(Equal(checkOptions.Status),
					"object %s/%s: condition %s status is %q, expected %q",
					u.GetNamespace(), u.GetName(), conditionType, actualStatus, checkOptions.Status)
			}
			if checkOptions.CheckMessage {
				g.Expect(actualMessage).To(Equal(checkOptions.Message),
					"object %s/%s: condition %s message is %q, expected %q",
					u.GetNamespace(), u.GetName(), conditionType, actualMessage, checkOptions.Message)
			}
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

// UntilObjectState waits for an object to reach the specified state.
// It accepts a runtime.Object (which serves as a template with name and namespace),
// expected state string, and timeout duration.
// The GVK is automatically extracted from the object via the client's scheme.
func UntilObjectState(expectedState string, timeout time.Duration, objs ...client.Object) {
	GinkgoHelper()
	untilObjectField("status.state", expectedState, timeout, objs...)
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
func untilObjectField(fieldPath, expectedValue string, timeout time.Duration, objs ...client.Object) {
	Eventually(func(g Gomega) {
		for _, obj := range objs {
			key := client.ObjectKeyFromObject(obj)
			name := obj.GetName()
			namespace := obj.GetNamespace()
			divider := ""
			if namespace != "" {
				divider = "/"
			}

			// Create a new unstructured object for each Get call
			u := getTemplateUnstructured(obj).DeepCopy()
			err := framework.GetClients().GenericClient().Get(context.Background(), key, u)
			if err != nil {
				g.Expect(err).NotTo(HaveOccurred(), "failed to get object %s%s%s", namespace, divider, name)
			}

			value := extractField(u, fieldPath)
			g.Expect(value).To(Equal(expectedValue), "object %s%s%s %s is %s, expected %s", namespace, divider, name, fieldPath, value, expectedValue)
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
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
