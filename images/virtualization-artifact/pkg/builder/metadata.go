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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectMetaBuilder struct {
	objectMeta metav1.ObjectMeta
}

func NewObjectMetaBuilder(name, namespace string) *ObjectMetaBuilder {
	return &ObjectMetaBuilder{
		objectMeta: NewObjectMeta(name, namespace),
	}
}

func (b *ObjectMetaBuilder) WithName(name string) *ObjectMetaBuilder {
	b.objectMeta.Name = name
	return b
}

func (b *ObjectMetaBuilder) WithNamespace(namespace string) *ObjectMetaBuilder {
	b.objectMeta.Namespace = namespace
	return b
}

func (b *ObjectMetaBuilder) WithLabel(key, value string) *ObjectMetaBuilder {
	if b.objectMeta.Labels == nil {
		b.objectMeta.Labels = make(map[string]string)
	}
	b.objectMeta.Labels[key] = value
	return b
}

func (b *ObjectMetaBuilder) WithAnnotation(key, value string) *ObjectMetaBuilder {
	if b.objectMeta.Annotations == nil {
		b.objectMeta.Annotations = make(map[string]string)
	}
	b.objectMeta.Annotations[key] = value
	return b
}

func (b *ObjectMetaBuilder) WithLabels(labels map[string]string) *ObjectMetaBuilder {
	b.objectMeta.Labels = labels
	return b
}

func (b *ObjectMetaBuilder) WithAnnotations(annotations map[string]string) *ObjectMetaBuilder {
	b.objectMeta.Annotations = annotations
	return b
}

func (b *ObjectMetaBuilder) Complete() metav1.ObjectMeta {
	return *b.objectMeta.DeepCopy()
}

func NewObjectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}
