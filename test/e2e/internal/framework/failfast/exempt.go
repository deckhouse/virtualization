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

package failfast

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Exemptions is the set of objects a suite deliberately drives into a
// dead-end state (e.g. an image with a spoiled checksum), shared by every
// fail-fast rule of a framework. An exempted object never fails the spec, and
// neither do the pods it owns: a provisioner pod crash-looping after the
// expected failure is part of the same picture, not a new one.
//
// The zero value is ready to use, and a nil *Exemptions exempts nothing.
type Exemptions struct {
	mu    sync.RWMutex
	names map[string]struct{}
	uids  map[types.UID]struct{}
}

// Add registers the objects as expected to fail. Registering before the
// object is created is the intended order: matching falls back to
// namespace/name, so the exemption is in force by the time the first watch
// event arrives, with no window for a rule to fire on the expected failure.
func (e *Exemptions) Add(objs ...metav1.Object) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.names == nil {
		e.names = make(map[string]struct{})
		e.uids = make(map[types.UID]struct{})
	}
	for _, obj := range objs {
		e.names[obj.GetNamespace()+"/"+obj.GetName()] = struct{}{}
		if uid := obj.GetUID(); uid != "" {
			e.uids[uid] = struct{}{}
		}
	}
}

// IsExempted reports whether the object is expected to fail: either it was
// registered itself, or it is owned by a registered object (the importer pod
// of an exempted image). An owner reference carries no namespace - an owner
// is namespaced together with its dependents - so the object's own namespace
// is used for the lookup.
func (e *Exemptions) IsExempted(obj metav1.Object) bool {
	if e == nil {
		return false
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.isExemptedLocked(obj.GetNamespace(), obj.GetName(), obj.GetUID()) {
		return true
	}
	for _, ref := range obj.GetOwnerReferences() {
		if e.isExemptedLocked(obj.GetNamespace(), ref.Name, ref.UID) {
			return true
		}
	}
	return false
}

func (e *Exemptions) isExemptedLocked(namespace, name string, uid types.UID) bool {
	if _, ok := e.names[namespace+"/"+name]; ok {
		return true
	}
	_, ok := e.uids[uid]
	return ok
}
