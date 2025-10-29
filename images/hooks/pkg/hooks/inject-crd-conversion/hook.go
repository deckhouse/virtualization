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

package inject_crd_conversion_cabundle

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"hooks/pkg/settings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	crdName                  = "virtualmachineclasses.virtualization.deckhouse.io"
	controllerTLSSecretName  = "virtualization-controller-tls"
	controllerDeploymentName = "virtualization-controller"
	deploymentSnapshotName   = "controller-deployment"
)

var _ = registry.RegisterFunc(config, reconcile)

var config = &pkg.HookConfig{
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       deploymentSnapshotName,
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{controllerDeploymentName},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
		},
	},
	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(ctx context.Context, input *pkg.HookInput) error {
	input.Logger.Info("Start inject CRD conversion webhook configuration hook")

	k8sClient, err := input.DC.GetK8sClient()
	if err != nil {
		return fmt.Errorf("get k8s client: %w", err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: crdName}, crd)
	if err != nil {
		input.Logger.Info("CRD not found, skipping conversion webhook injection", slog.Any("error", err))
		return nil
	}

	if crd.Spec.Conversion != nil && crd.Spec.Conversion.Strategy == apiextensionsv1.WebhookConverter {
		input.Logger.Info("CRD already has webhook conversion configured, skipping")
		return nil
	}

	secret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: settings.ModuleNamespace,
		Name:      controllerTLSSecretName,
	}, secret)
	if err != nil {
		input.Logger.Info("Controller TLS secret not found, skipping conversion webhook injection", slog.Any("error", err))
		return nil
	}

	caBundle, ok := secret.Data["ca.crt"]
	if !ok || len(caBundle) == 0 {
		return fmt.Errorf("CA certificate is empty in controller TLS secret")
	}

	conversionConfig := map[string]interface{}{
		"strategy": "Webhook",
		"webhook": map[string]interface{}{
			"clientConfig": map[string]interface{}{
				"service": map[string]interface{}{
					"name":      "virtualization-controller",
					"namespace": "d8-virtualization",
					"path":      "/convert",
					"port":      443,
				},
				"caBundle": base64.StdEncoding.EncodeToString(caBundle),
			},
			"conversionReviewVersions": []string{"v1"},
		},
	}

	patch := []interface{}{
		map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/conversion",
			"value": conversionConfig,
		},
	}

	input.PatchCollector.JSONPatch(patch, "apiextensions.k8s.io/v1", "CustomResourceDefinition", "", crdName)
	input.Logger.Info("Successfully injected conversion webhook configuration into CRD", slog.String("crd", crdName))

	return nil
}
