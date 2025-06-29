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
	return de.getStatus()["publicURL"].(string)
}

func (de *DataExport) GetStatusConditions() []metav1.Condition {
	return de.getStatus()["conditions"].([]metav1.Condition)
}

func (de *DataExport) GetStatusAccessTimestamp() metav1.Time {
	return de.getStatus()["accessTimestamp"].(metav1.Time)
}

func (de *DataExport) getStatus() map[string]interface{} {
	return de.Unstructured.Object["status"].(map[string]interface{})
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
