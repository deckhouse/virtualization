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

package service

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
)

// DefaultCleanUpReason is the reason reported while a resource cleanup is still
// in progress but no more specific reason was produced. Like every reason built by
// CleanUpReason it is a lowercase clause: it may be merged with other reasons, and
// only the first letter of the whole message is capitalized by SetTerminatingCondition.
const DefaultCleanUpReason = "waiting for the auxiliary resources to be deleted"

// Cleanup roles describe the auxiliary resources created while a disk or an image is
// provisioned. A role is always reported together with the resource name: the name
// alone does not tell an importer Pod from an uploader Ingress, and the role alone
// does not tell which object to inspect. With both, the user can go straight to
// `d8 k describe <object>` and see what actually holds the deletion (a finalizer, an
// unreachable node, a volume still attached).
const (
	CleanUpRoleImporterPod           = "importer Pod"
	CleanUpRoleUploaderPod           = "uploader Pod"
	CleanUpRoleBounderPod            = "bounder Pod"
	CleanUpRoleUploaderService       = "uploader Service"
	CleanUpRoleUploaderIngress       = "uploader Ingress"
	CleanUpRoleUploaderHTTPRoute     = "uploader HTTPRoute"
	CleanUpRoleNetworkPolicy         = "NetworkPolicy"
	CleanUpRolePersistentVolumeClaim = "PersistentVolumeClaim"
)

// maxConditionMessageLength bounds the length of the condition messages built by this
// package. Kubernetes limits condition.message to 32768 characters (metav1.Condition
// kubebuilder MaxLength); exceeding it makes the API server reject the status
// update and the controller gets stuck. We keep the message well below that so it
// stays readable in kubectl/UI even when many VirtualMachines are involved.
const maxConditionMessageLength = 256

// CleanUpReason builds a cleanup progress reason naming the auxiliary resource that is
// still being deleted, e.g.
//
//	waiting for the importer Pod "default/vd-importer-mydisk" to be deleted
//
// The reason is a lowercase clause: several of them are merged by MergeCleanUpReasons
// and capitalized by SetTerminatingCondition.
func CleanUpReason(role string, name types.NamespacedName) string {
	if role == "" {
		return ""
	}

	if name.Name == "" {
		return fmt.Sprintf("waiting for the %s to be deleted", role)
	}

	target := name.Name
	if name.Namespace != "" {
		target = name.Namespace + "/" + name.Name
	}

	return fmt.Sprintf("waiting for the %s %q to be deleted", role, target)
}

// CleanUpReasonForObject is CleanUpReason for an already fetched object.
func CleanUpReasonForObject(role string, obj client.Object) string {
	if obj == nil {
		return ""
	}

	return CleanUpReason(role, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()})
}

func MergeCleanUpReasons(reasons ...string) string {
	var merged []string
	seen := make(map[string]struct{}, len(reasons))

	for _, reason := range reasons {
		if reason == "" {
			continue
		}

		if _, ok := seen[reason]; ok {
			continue
		}

		seen[reason] = struct{}{}
		merged = append(merged, reason)
	}

	return strings.Join(merged, "; ")
}

// SetTerminatingCondition sets a Terminating condition with the provided reason and
// message. The status is True: the condition exists only while the object is being
// deleted, and it stays True for as long as the deletion is in progress. The message
// is capitalized, terminated with a period and truncated to maxConditionMessageLength.
func SetTerminatingCondition(conds *[]metav1.Condition, conditionType, reason conditions.Stringer, generation int64, message string) {
	conditions.SetCondition(
		conditions.NewConditionBuilder(conditionType).
			Generation(generation).
			Status(metav1.ConditionTrue).
			Reason(reason).
			Message(truncateMessage(CapitalizeFirstLetter(message)+".")),
		conds,
	)
}

// truncationMarker replaces the tail that did not fit. It ends the sentence itself,
// because the message contract (capitalized, terminated with a period) must hold for
// truncated messages too: a bare ellipsis would eat the terminating period and read
// like a word cut in half.
const truncationMarker = " (truncated)."

// truncateMessage shortens s to at most maxConditionMessageLength runes, replacing the
// trimmed tail with truncationMarker.
//
// Every message built here is a list of clauses or of quoted names, so the tail is dropped
// whole: a cut in the middle of a clause leaves a half-typed word, and a cut inside a
// quoted name leaves an unbalanced quote and half of a "namespace/name" the user cannot
// paste into `d8 k describe`.
func truncateMessage(s string) string {
	runes := []rune(s)
	if len(runes) <= maxConditionMessageLength {
		return s
	}

	limit := maxConditionMessageLength - len([]rune(truncationMarker))
	hardCut := string(runes[:limit])
	kept := hardCut

	// Prefer to drop the whole trailing clause or list item; fall back to a word
	// boundary when the tail holds no separator at all.
	cut := max(strings.LastIndex(kept, "; "), strings.LastIndex(kept, ", "))
	if cut < 0 {
		cut = strings.LastIndex(kept, " ")
	}
	if cut > 0 {
		kept = kept[:cut]
	}

	// Safety net for the word-boundary fallback: an odd number of quotes means the cut
	// still landed inside a quoted name, so drop it from its opening quote.
	if strings.Count(kept, `"`)%2 != 0 {
		if openingQuote := strings.LastIndex(kept, `"`); openingQuote >= 0 {
			kept = kept[:openingQuote]
		}
	}

	kept = strings.TrimRight(kept, " ,;")
	if kept == "" {
		// Nothing survived the cut (a message that opens with a quoted name longer than
		// the whole limit): keep the hard cut rather than reporting only the marker.
		kept = hardCut
	}

	return kept + truncationMarker
}
