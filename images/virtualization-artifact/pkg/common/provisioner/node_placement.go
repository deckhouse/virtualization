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

package provisioner

import (
	"encoding/base64"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

type NodePlacement struct {
	// tolerations is a list of tolerations applied to the relevant kind of pods
	// See https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/ for more info.
	// These are additional tolerations other than default ones.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

func IsNodePlacementChanged(nodePlacement *NodePlacement, obj client.Object) (bool, error) {
	oldHash, exists := obj.GetAnnotations()[annotations.AnnTolerationsHash]

	if nodePlacement == nil && exists {
		return true, nil
	}

	if (nodePlacement == nil || len(nodePlacement.Tolerations) == 0) && !exists {
		return false, nil
	}

	JSON, err := json.Marshal(nodePlacement.Tolerations)
	if err != nil {
		return false, err
	}

	newHash := base64.StdEncoding.EncodeToString(JSON)

	return oldHash != newHash, nil
}

func KeepNodePlacementTolerations(nodePlacement *NodePlacement, obj client.Object) error {
	anno := obj.GetAnnotations()

	if nodePlacement == nil || len(nodePlacement.Tolerations) == 0 {
		_, ok := anno[annotations.AnnTolerationsHash]
		if !ok {
			return nil
		}

		delete(anno, annotations.AnnTolerationsHash)

		obj.SetAnnotations(anno)

		return nil
	}

	JSON, err := json.Marshal(nodePlacement.Tolerations)
	if err != nil {
		return err
	}

	if anno == nil {
		anno = make(map[string]string)
	}

	anno[annotations.AnnTolerationsHash] = base64.StdEncoding.EncodeToString(JSON)

	obj.SetAnnotations(anno)

	return nil
}

var systemNodeToleration = corev1.Toleration{
	Key:      "dedicated.deckhouse.io",
	Operator: corev1.TolerationOpEqual,
	Value:    "system",
}

func AddTolerationForSystemNodes(placement *NodePlacement) {
	if placement == nil {
		return
	}
	// Do nothing if system-node toleration is present.
	for _, toleration := range placement.Tolerations {
		if toleration.Key == systemNodeToleration.Key && toleration.Value == systemNodeToleration.Value {
			return
		}
	}
	placement.Tolerations = append(placement.Tolerations, systemNodeToleration)
}
