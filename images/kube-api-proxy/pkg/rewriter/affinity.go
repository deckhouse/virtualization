/*
Copyright 2024 Flant JSC

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

package rewriter

import (
	"kube-api-proxy/pkg/rewriter/rules"
	"kube-api-proxy/pkg/rewriter/transform"
)

// RewriteAffinity renames or restores labels in labelSelector of affinity structure.
// See https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity
func RewriteAffinity(rwRules *rules.RewriteRules, obj []byte, path string, action rules.Action) ([]byte, error) {
	return transform.Object(obj, path, func(affinity []byte) ([]byte, error) {
		rwrAffinity, err := transform.Object(affinity, "nodeAffinity", func(item []byte) ([]byte, error) {
			return rewriteNodeAffinity(rwRules, item, action)
		})
		if err != nil {
			return nil, err
		}

		rwrAffinity, err = transform.Object(rwrAffinity, "podAffinity", func(item []byte) ([]byte, error) {
			return rewritePodAffinity(rwRules, item, action)
		})
		if err != nil {
			return nil, err
		}

		return transform.Object(rwrAffinity, "podAntiAffinity", func(item []byte) ([]byte, error) {
			return rewritePodAffinity(rwRules, item, action)
		})

	})
}

// rewriteNodeAffinity rewrites labels in nodeAffinity structure.
// nodeAffinity:
//
//	requiredDuringSchedulingIgnoredDuringExecution:
//	  nodeSelectorTerms []NodeSelector -> rewrite each item: key in each matchExpressions and matchFields
//	preferredDuringSchedulingIgnoredDuringExecution: -> array of PreferredSchedulingTerm:
//	  preference NodeSelector ->  rewrite key in each matchExpressions and matchFields
//	  weight:
func rewriteNodeAffinity(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	// Rewrite an array of nodeSelectorTerms in requiredDuringSchedulingIgnoredDuringExecution field.
	var err error
	obj, err = transform.Object(obj, "requiredDuringSchedulingIgnoredDuringExecution", func(affinityTerm []byte) ([]byte, error) {
		return transform.Array(affinityTerm, "nodeSelectorTerms", func(item []byte) ([]byte, error) {
			return rewriteNodeSelectorTerm(rwRules, item, action)
		})
	})
	if err != nil {
		return nil, err
	}

	// Rewrite an array of weightedNodeSelectorTerms in preferredDuringSchedulingIgnoredDuringExecution field.
	return transform.Array(obj, "preferredDuringSchedulingIgnoredDuringExecution", func(item []byte) ([]byte, error) {
		return transform.Object(item, "preference", func(preference []byte) ([]byte, error) {
			return rewriteNodeSelectorTerm(rwRules, preference, action)
		})
	})
}

// rewriteNodeSelectorTerm renames or restores key fields in matchLabels or matchExpressions of NodeSelectorTerm.
func rewriteNodeSelectorTerm(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	obj, err := transform.Array(obj, "matchLabels", func(item []byte) ([]byte, error) {
		return rewriteSelectorRequirement(rwRules, item, action)
	})
	if err != nil {
		return nil, err
	}
	return transform.Array(obj, "matchExpressions", func(item []byte) ([]byte, error) {
		return rewriteSelectorRequirement(rwRules, item, action)
	})
}

func rewriteSelectorRequirement(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	return transform.String(obj, "key", func(field string) string {
		return rwRules.LabelsRewriter().Rewrite(field, action)
	})
}

// rewritePodAffinity rewrites PodAffinity and PodAntiAffinity structures.
// PodAffinity and PodAntiAffinity structures are the same:
//
//	requiredDuringSchedulingIgnoredDuringExecution -> array of PodAffinityTerm structures:
//	  labelSelector:
//	    matchLabels -> rewrite map
//	    matchExpressions -> rewrite key in each item
//	  topologyKey -> rewrite as label name
//	  namespaceSelector -> rewrite as labelSelector
//	preferredDuringSchedulingIgnoredDuringExecution -> array of WeightedPodAffinityTerm:
//	  weight
//	  podAffinityTerm PodAffinityTerm -> rewrite as described above
func rewritePodAffinity(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	// Rewrite an array of PodAffinityTerms in requiredDuringSchedulingIgnoredDuringExecution field.
	obj, err := transform.Array(obj, "requiredDuringSchedulingIgnoredDuringExecution", func(affinityTerm []byte) ([]byte, error) {
		return rewritePodAffinityTerm(rwRules, affinityTerm, action)
	})
	if err != nil {
		return nil, err
	}

	// Rewrite an array of WeightedPodAffinityTerms in requiredDuringSchedulingIgnoredDuringExecution field.
	return transform.Array(obj, "preferredDuringSchedulingIgnoredDuringExecution", func(affinityTerm []byte) ([]byte, error) {
		return transform.Object(affinityTerm, "podAffinityTerm", func(podAffinityTerm []byte) ([]byte, error) {
			return rewritePodAffinityTerm(rwRules, podAffinityTerm, action)
		})
	})
}

func rewritePodAffinityTerm(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	obj, err := transform.Object(obj, "labelSelector", func(labelSelector []byte) ([]byte, error) {
		return RewriteLabelSelector(rwRules, labelSelector, action)
	})
	if err != nil {
		return nil, err
	}

	obj, err = transform.String(obj, "topologyKey", func(field string) string {
		return rwRules.LabelsRewriter().Rewrite(field, action)
	})
	if err != nil {
		return nil, err
	}

	return transform.Object(obj, "namespaceSelector", func(selector []byte) ([]byte, error) {
		return RewriteLabelSelector(rwRules, selector, action)
	})
}

// RewriteLabelSelector rewrites matchLabels and matchExpressions. It is similar to rewriteNodeSelectorTerm
// but matchLabels is a map here, not an array of requirements.
func RewriteLabelSelector(rwRules *rules.RewriteRules, obj []byte, action rules.Action) ([]byte, error) {
	obj, err := RewriteLabelsMap(rwRules, obj, "matchLabels", action)
	if err != nil {
		return nil, err
	}

	return transform.Array(obj, "matchExpressions", func(item []byte) ([]byte, error) {
		return rewriteSelectorRequirement(rwRules, item, action)
	})
}
