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

package restore

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// restoreBreakingAnnotations are server/scheduler-managed annotations that must not survive into
// restore output (see the unified-snapshotter overview ADR, "Restore" section).
var restoreBreakingAnnotations = []string{
	"kubectl.kubernetes.io/last-applied-configuration",
	"pv.kubernetes.io/bind-completed",
	"pv.kubernetes.io/bound-by-controller",
	"volume.kubernetes.io/selected-node",
}

// SanitizeForRestore returns an apply-ready copy of obj rewritten into targetNamespace: it strips
// status, runtime-managed metadata, and restore-breaking annotations, and rewrites metadata.namespace.
//
// Finalizers are DELIBERATELY preserved: they encode intent (or are harmlessly re-added by the target
// cluster's own controllers) and must survive a restore, mirroring the core's own restore-path
// sanitizer. ownerReferences ARE stripped: a dangling ownerReference (its owner UID absent in the
// target namespace) would make the API server garbage-collect the restored object immediately.
func SanitizeForRestore(obj unstructured.Unstructured, targetNamespace string) unstructured.Unstructured {
	out := obj.DeepCopy()
	unstructured.RemoveNestedField(out.Object, "status")
	for _, f := range []string{
		"uid", "resourceVersion", "generation", "creationTimestamp",
		"deletionTimestamp", "deletionGracePeriodSeconds", "managedFields",
		"ownerReferences", "selfLink",
	} {
		unstructured.RemoveNestedField(out.Object, "metadata", f)
	}
	stripRestoreBreakingAnnotations(out)
	out.SetNamespace(targetNamespace)
	return *out
}

func stripRestoreBreakingAnnotations(out *unstructured.Unstructured) {
	anns, found, err := unstructured.NestedMap(out.Object, "metadata", "annotations")
	if err != nil || !found || len(anns) == 0 {
		return
	}
	for _, k := range restoreBreakingAnnotations {
		delete(anns, k)
	}
	if len(anns) == 0 {
		unstructured.RemoveNestedField(out.Object, "metadata", "annotations")
		return
	}
	_ = unstructured.SetNestedMap(out.Object, anns, "metadata", "annotations")
}
