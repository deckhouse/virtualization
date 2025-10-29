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

package crd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	vmClassCRDName = "virtualmachineclasses.virtualization.deckhouse.io"
	tlsCertPath    = "/tmp/k8s-webhook-server/serving-certs/ca.crt"
)

// EnsureVMClassConversionWebhook configures the VirtualMachineClass CRD to use webhook-based conversion.
// This allows Kubernetes to convert between v1alpha2 (storage version) and v1alpha3 (served version).
// Returns nil if conversion is already configured, otherwise returns an error.
func EnsureVMClassConversionWebhook(ctx context.Context, c client.Client, controllerNamespace string) error {
	logger := log.FromContext(ctx).WithName("crd-conversion")

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, client.ObjectKey{Name: vmClassCRDName}, crd); err != nil {
		return fmt.Errorf("get VirtualMachineClass CRD: %w", err)
	}

	caBytes, err := os.ReadFile(tlsCertPath)
	if err != nil {
		return fmt.Errorf("read TLS CA certificate from %s: %w", tlsCertPath, err)
	}

	caBundle := base64.StdEncoding.EncodeToString(caBytes)

	crd.Spec.Conversion = &apiextensionsv1.CustomResourceConversion{
		Strategy: apiextensionsv1.WebhookConverter,
		Webhook: &apiextensionsv1.WebhookConversion{
			ClientConfig: &apiextensionsv1.WebhookClientConfig{
				Service: &apiextensionsv1.ServiceReference{
					Name:      "virtualization-controller",
					Namespace: controllerNamespace,
					Path:      ptr.To("/convert"),
					Port:      ptr.To[int32](443),
				},
				CABundle: []byte(caBundle),
			},
			ConversionReviewVersions: []string{"v1"},
		},
	}

	if err := c.Update(ctx, crd); err != nil {
		logger.Error(err, "Failed to update VirtualMachineClass CRD with conversion webhook configuration")
		return fmt.Errorf("update CRD with conversion webhook: %w", err)
	}

	logger.Info("Successfully configured VirtualMachineClass CRD with webhook conversion")
	return nil
}
