package storage

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

func nameFor(fs fields.Selector) (string, error) {
	if fs == nil {
		fs = fields.Everything()
	}
	name, found := fs.RequiresExactMatch("metadata.name")
	if !found && !fs.Empty() {
		return "", fmt.Errorf("field label not supported: %s", fs.Requirements()[0].Field)
	}
	return name, nil
}

func matches(obj metav1.Object, name string) bool {
	if name == "" {
		name = obj.GetName()
	}
	return obj.GetName() == name
}
