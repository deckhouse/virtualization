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

package gc

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: "fake.io", Version: "v1"}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&FakeObject{},
		&FakeObjectList{},
	)
	return nil
}

func NewFakeObject(name, namespace string) *FakeObject {
	obj := NewEmptyFakeObject()
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func NewEmptyFakeObject() *FakeObject {
	return &FakeObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "FakeObject",
		},
	}
}

type FakeObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	RefObject string `json:"refObject"`
	Phase     string `json:"phase"`
}

func (f FakeObject) DeepCopyObject() runtime.Object {
	return &FakeObject{
		TypeMeta:   f.TypeMeta,
		ObjectMeta: *f.ObjectMeta.DeepCopy(),
		RefObject:  f.RefObject,
		Phase:      f.Phase,
	}
}

type FakeObjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []FakeObject `json:"items"`
}

func (f *FakeObjectList) DeepCopyObject() runtime.Object {
	if f == nil {
		return nil
	}

	items := make([]FakeObject, len(f.Items))
	for i := range f.Items {
		items[i] = *f.Items[i].DeepCopyObject().(*FakeObject)
	}

	return &FakeObjectList{
		TypeMeta: f.TypeMeta,
		ListMeta: *f.ListMeta.DeepCopy(),
		Items:    items,
	}
}

const (
	fakeObjectPhasePending   = "Pending"
	fakeObjectPhaseRunning   = "Running"
	fakeObjectPhaseCompleted = "Completed"
)

var _ ReconcileGCManager = &fakeGCManager{}

func newFakeGCManager(client client.Client, ttl time.Duration, max int) *fakeGCManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	if max == 0 {
		max = 10
	}
	return &fakeGCManager{
		client: client,
		ttl:    ttl,
		max:    max,
	}
}

type fakeGCManager struct {
	client client.Client
	ttl    time.Duration
	max    int
}

func (f *fakeGCManager) New() client.Object {
	return &FakeObject{}
}

func (f *fakeGCManager) ShouldBeDeleted(obj client.Object) bool {
	fobj, ok := obj.(*FakeObject)
	if !ok {
		return false
	}
	return fobj.Phase == fakeObjectPhaseCompleted
}

func (f *fakeGCManager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	fobjList := &FakeObjectList{}
	err := f.client.List(ctx, fobjList)
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0, len(fobjList.Items))
	for _, obj := range fobjList.Items {
		objs = append(objs, &obj)
	}

	result := DefaultFilter(objs, f.ShouldBeDeleted, f.ttl, f.getIndex, f.max, now)

	return result, nil
}

func (f *fakeGCManager) getIndex(obj client.Object) string {
	fobj, ok := obj.(*FakeObject)
	if !ok {
		return ""
	}
	return fobj.RefObject
}
