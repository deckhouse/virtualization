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

package kubeapi

import (
	"log/slog"
	"os"
	"sync"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	kubeClient     kubernetes.Interface
	kubeClientOnce sync.Once
)

func getClient() kubernetes.Interface {
	kubeClientOnce.Do(func() {
		restConfig, err := config.GetConfig()
		if err != nil {
			slog.Error("failed to get kubeconfig", "error", err)
			return
		}

		kubeClient, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			slog.Error("failed to create kubernetes client", "error", err)
		}
	})

	return kubeClient
}

func ResourceV1Available() bool {
	client := getClient()
	if client == nil {
		return false
	}

	enabled, err := isResourceV1Enabled(client)
	if err != nil {
		slog.Error("failed to check if resource v1 is enabled", "error", err)
	}
	return enabled
}

func isResourceV1Enabled(clientset kubernetes.Interface) (bool, error) {
	_, apis, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return false, err
	}

	for _, api := range apis {
		if api.GroupVersion == resourcev1.SchemeGroupVersion.String() {
			return true, nil
		}
	}

	return false, nil
}

func HasDRAFeatureGates() bool {
	envValue := os.Getenv("HAS_DRA_FEATURE_GATES")
	if envValue == "" {
		return false
	}

	return envValue == "true"
}
