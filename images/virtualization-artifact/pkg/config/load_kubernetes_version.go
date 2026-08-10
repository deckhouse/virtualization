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

package config

import (
	"fmt"
	"os"

	k8sversion "k8s.io/apimachinery/pkg/util/version"
)

// KubernetesVersionVar is an env variable holds global discovery kubernetesVersion value.
const KubernetesVersionVar = "KUBERNETES_VERSION"

// LoadKubernetesVersionFromEnv reads the cluster Kubernetes version once at start up. The version
// only gates features by their minimal Kubernetes release, so a value discovered by Deckhouse is
// enough: querying the API server on every admission request would tie the webhook to discovery.
func LoadKubernetesVersionFromEnv() (*k8sversion.Version, error) {
	raw := os.Getenv(KubernetesVersionVar)
	if raw == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify global discovery kubernetesVersion from cluster configuration", KubernetesVersionVar)
	}

	version, err := k8sversion.ParseGeneric(raw)
	if err != nil {
		return nil, fmt.Errorf("parse kubernetesVersion %q: %w", raw, err)
	}

	return version, nil
}
