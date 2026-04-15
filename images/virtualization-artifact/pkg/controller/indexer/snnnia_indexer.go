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

package indexer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var snnniaGVK = schema.GroupVersionKind{
	Group:   "network.deckhouse.io",
	Version: "v1alpha1",
	Kind:    "SystemNetworkNodeNetworkInterfaceAttachment",
}

func snnniaSeed() client.Object {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(snnniaGVK)
	return u
}

func snnniaIndexer(path ...string) client.IndexerFunc {
	return func(o client.Object) []string {
		u, ok := o.(*unstructured.Unstructured)
		if !ok || u == nil {
			return nil
		}
		v, _, _ := unstructured.NestedString(u.Object, path...)
		if v == "" {
			return nil
		}
		return []string{v}
	}
}

func IndexSNNNIAByNodeName() (client.Object, string, client.IndexerFunc) {
	return snnniaSeed(), IndexFieldSNNNIAByNodeName, snnniaIndexer("status", "nodeName")
}

func IndexSNNNIABySystemNetworkName() (client.Object, string, client.IndexerFunc) {
	return snnniaSeed(), IndexFieldSNNNIABySystemNetworkName, snnniaIndexer("spec", "systemNetworkName")
}
