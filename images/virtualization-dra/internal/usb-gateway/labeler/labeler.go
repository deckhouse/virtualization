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

package labeler

import (
	"context"
	"fmt"
	"maps"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/deckhouse/virtualization-dra/pkg/patch"
)

type Labeler interface {
	Label(ctx context.Context, name, namespace string, addLabels map[string]string, removeLabels []string) error
}

type genericLabeler struct {
	client dynamic.Interface
	gvr    schema.GroupVersionResource
}

func NewGenericLabeler(client dynamic.Interface, gvr schema.GroupVersionResource) Labeler {
	return &genericLabeler{
		client: client,
		gvr:    gvr,
	}
}

func (l *genericLabeler) Label(ctx context.Context, name, namespace string, addLabels map[string]string, removeLabels []string) error {
	if addLabels == nil && removeLabels == nil {
		return nil
	}

	obj, err := l.client.Resource(l.gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	oldLabels := obj.GetLabels()
	newLabels := make(map[string]string)
	maps.Copy(newLabels, oldLabels)
	for _, k := range removeLabels {
		delete(newLabels, k)
	}
	maps.Copy(newLabels, addLabels)

	if equality.Semantic.DeepEqual(oldLabels, newLabels) {
		return nil
	}

	patchBytes, err := patch.NewJSONPatch(
		patch.WithTest("/metadata/labels", oldLabels),
		patch.WithReplace("/metadata/labels", newLabels),
	).Bytes()
	if err != nil {
		return fmt.Errorf("failed to create patch: %w", err)
	}

	_, err = l.client.Resource(l.gvr).Namespace(namespace).Patch(ctx, name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	return err
}

type NodeLabeler struct {
	generic Labeler
}

func NewNodeLabeler(client dynamic.Interface) NodeLabeler {
	return NodeLabeler{
		generic: NewGenericLabeler(client, schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "nodes",
		}),
	}
}

func (l NodeLabeler) Label(ctx context.Context, name, namespace string, addLabels map[string]string, removeLabels []string) error {
	return l.generic.Label(ctx, name, namespace, addLabels, removeLabels)
}
