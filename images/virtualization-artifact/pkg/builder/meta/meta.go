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

package meta

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func WithName[T client.Object](name string) func(obj T) {
	return func(obj T) {
		obj.SetName(name)
	}
}

func WithNamespace[T client.Object](namespace string) func(obj T) {
	return func(obj T) {
		obj.SetNamespace(namespace)
	}
}

func WithGenerateName[T client.Object](generateName string) func(obj T) {
	return func(obj T) {
		obj.SetGenerateName(generateName)
	}
}

func WithLabels[T client.Object](labels map[string]string) func(obj T) {
	return func(obj T) {
		obj.SetLabels(labels)
	}
}

func WithLabel[T client.Object](key, value string) func(obj T) {
	return func(obj T) {
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[key] = value
		obj.SetLabels(labels)
	}
}

func WithAnnotations[T client.Object](annotations map[string]string) func(obj T) {
	return func(obj T) {
		obj.SetAnnotations(annotations)
	}
}

func WithAnnotation[T client.Object](key, value string) func(obj T) {
	return func(obj T) {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[key] = value
		obj.SetAnnotations(annotations)
	}
}

func WithFinalizer[T client.Object](finalizer string) func(obj T) {
	return func(obj T) {
		controllerutil.AddFinalizer(obj, finalizer)
	}
}
