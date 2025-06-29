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

package dataexport

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewEmptyDataExport() *DataExport {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "storage.deckhouse.io",
		Version: "v1alpha1",
		Kind:    "DataExport",
	})
	return &DataExport{Unstructured: u}
}

type DataExport struct {
	*unstructured.Unstructured
}

func (de *DataExport) GetStatusPublicURL() string {
	if status := de.getStatus(); status != nil {
		if val, ok := status["publicURL"].(string); ok {
			return val
		}
	}
	return ""
}

func (de *DataExport) GetStatusConditions() []metav1.Condition {
	if status := de.getStatus(); status != nil {
		if val, ok := status["conditions"].([]metav1.Condition); ok {
			return val
		}
	}
	return nil
}

func (de *DataExport) GetStatusAccessTimestamp() metav1.Time {
	if status := de.getStatus(); status != nil {
		if val, ok := status["accessTimestamp"].(metav1.Time); ok {
			return val
		}
	}
	return metav1.Time{}
}

func (de *DataExport) getStatus() map[string]interface{} {
	if de.Unstructured.Object == nil {
		return nil
	}
	status, _ := de.Unstructured.Object["status"].(map[string]interface{})
	return status
}

func (de *DataExport) setEmptySpecIfNeeded() {
	if spec := de.Unstructured.Object["spec"]; spec == nil {
		de.Unstructured.Object["spec"] = make(map[string]interface{})
	}
}

func (de *DataExport) getSpec() map[string]interface{} {
	return de.Unstructured.Object["spec"].(map[string]interface{})
}

func (de *DataExport) SetTargetRef(kind, name string) {
	de.setEmptySpecIfNeeded()
	de.getSpec()["targetRef"] = map[string]string{
		"kind": kind,
		"name": name,
	}
}

func (de *DataExport) SetTTL(ttl string) {
	de.setEmptySpecIfNeeded()
	de.getSpec()["ttl"] = ttl
}

func (de *DataExport) SetPublish() {
	de.setEmptySpecIfNeeded()
	de.getSpec()["publish"] = true
}
