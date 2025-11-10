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

package rewrite

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type Client interface {
	Get(ctx context.Context, name string, obj Object, opts ...Option) error
}

func NewRewriteClient(dynamicClient dynamic.Interface) Client {
	return rewriteClient{
		dynamicClient: dynamicClient,
	}
}

type rewriteClient struct {
	dynamicClient dynamic.Interface
}

type Object interface {
	GVR() schema.GroupVersionResource
}

type options struct {
	namespace string
}

type Option func(*options)

func InNamespace(namespace string) Option {
	return func(o *options) {
		o.namespace = namespace
	}
}

func makeOptions(opts ...Option) options {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func (r rewriteClient) Get(ctx context.Context, name string, obj Object, opts ...Option) error {
	o := makeOptions(opts...)

	var (
		u   *unstructured.Unstructured
		err error
	)

	if o.namespace != "" {
		u, err = r.dynamicClient.Resource(obj.GVR()).Namespace(o.namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		u, err = r.dynamicClient.Resource(obj.GVR()).Get(ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		return err
	}

	bytes, err := json.Marshal(u)
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, obj)
}
