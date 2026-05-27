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

package cdi_cleanup

import (
	"context"
	"fmt"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const cdiCleanupNamespace = settings.ModuleNamespace

var _ = registry.RegisterFunc(config, cleanup)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 15},
	Queue:        fmt.Sprintf("modules/%s", settings.ModuleName),
}

type staleResource struct {
	apiVersion string
	kind       string
	namespace  string
	name       string
}

func cleanup(_ context.Context, input *pkg.HookInput) error {
	for _, resource := range staleCDIResources() {
		input.PatchCollector.DeleteInBackground(resource.apiVersion, resource.kind, resource.namespace, resource.name)
	}

	return nil
}

func staleCDIResources() []staleResource {
	resources := make([]staleResource, 0, 64)

	resources = append(resources, cdiOperatorResources()...)
	resources = append(resources, cdiAdmissionResources()...)
	resources = append(resources, cdiRuntimeResources()...)
	resources = append(resources, cdiCRDResources()...)

	return resources
}

func cdiOperatorResources() []staleResource {
	return []staleResource{
		namespaced("apps/v1", "Deployment", "cdi-operator"),
		namespaced("policy/v1", "PodDisruptionBudget", "cdi-operator"),
		namespaced("autoscaling.k8s.io/v1", "VerticalPodAutoscaler", "cdi-operator"),
	}
}

func cdiAdmissionResources() []staleResource {
	return []staleResource{
		cluster("admissionregistration.k8s.io/v1", "MutatingWebhookConfiguration", "cdi-internal-virtualization-api-datavolume-mutate"),
		cluster("admissionregistration.k8s.io/v1", "MutatingWebhookConfiguration", "cdi-internal-virtualization-api-pvc-mutate"),
		cluster("admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration", "cdi-internal-virtualization-api-dataimportcron-validate"),
		cluster("admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration", "cdi-internal-virtualization-api-populator-validate"),
		cluster("admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration", "cdi-internal-virtualization-api-datavolume-validate"),
		cluster("admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration", "cdi-internal-virtualization-api-validate"),
		cluster("admissionregistration.k8s.io/v1", "ValidatingWebhookConfiguration", "cdi-internal-virtualization-objecttransfer-api-validate"),
		cluster("apiregistration.k8s.io/v1", "APIService", "v1beta1.upload.cdi.kubevirt.io"),
		cluster("apiregistration.k8s.io/v1", "APIService", "v1beta1.upload.cdi.internal.virtualization.deckhouse.io"),
		cluster("rbac.authorization.k8s.io/v1", "ClusterRole", "d8:containerized-data-importer:cdi-operator"),
		cluster("rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "d8:containerized-data-importer:cdi-operator"),
		cluster("rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "d8:virtualization:cdi-operator-rbac-proxy"),
		cluster("rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "d8:virtualization:cdi-deployment-rbac-proxy"),
	}
}

func cdiRuntimeResources() []staleResource {
	return []staleResource{
		namespaced("apps/v1", "Deployment", "cdi-apiserver"),
		namespaced("apps/v1", "Deployment", "cdi-deployment"),
		namespaced("v1", "Service", "cdi-api"),
		namespaced("v1", "Service", "cdi-prometheus-metrics"),
		namespaced("v1", "Service", "cdi-uploadproxy"),
		namespaced("v1", "ConfigMap", "cdi-operator-leader-election-helper"),
		namespaced("v1", "ConfigMap", "cdi-apiserver-signer-bundle"),
		namespaced("v1", "ConfigMap", "cdi-uploadproxy-signer-bundle"),
		namespaced("v1", "ConfigMap", "cdi-uploadserver-signer-bundle"),
		namespaced("v1", "ConfigMap", "cdi-uploadserver-client-signer-bundle"),
		namespaced("v1", "Secret", "cdi-apiserver-signer"),
		namespaced("v1", "Secret", "cdi-apiserver-server-cert"),
		namespaced("v1", "Secret", "cdi-uploadproxy-signer"),
		namespaced("v1", "Secret", "cdi-uploadproxy-server-cert"),
		namespaced("v1", "Secret", "cdi-uploadserver-signer"),
		namespaced("v1", "Secret", "cdi-uploadserver-client-signer"),
		namespaced("v1", "Secret", "cdi-uploadserver-client-cert"),
		namespaced("v1", "ServiceAccount", "cdi-operator"),
		namespaced("v1", "ServiceAccount", "cdi-apiserver"),
		namespaced("v1", "ServiceAccount", "cdi-sa"),
		namespaced("rbac.authorization.k8s.io/v1", "Role", "cdi-operator"),
		namespaced("rbac.authorization.k8s.io/v1", "Role", "cdi-apiserver"),
		namespaced("rbac.authorization.k8s.io/v1", "Role", "cdi-deployment"),
		namespaced("rbac.authorization.k8s.io/v1", "RoleBinding", "cdi-operator"),
		namespaced("rbac.authorization.k8s.io/v1", "RoleBinding", "cdi-apiserver"),
		namespaced("rbac.authorization.k8s.io/v1", "RoleBinding", "cdi-deployment"),
		namespaced("policy/v1", "PodDisruptionBudget", "cdi-apiserver"),
		namespaced("policy/v1", "PodDisruptionBudget", "cdi-deployment"),
		namespaced("autoscaling.k8s.io/v1", "VerticalPodAutoscaler", "cdi-apiserver"),
		namespaced("autoscaling.k8s.io/v1", "VerticalPodAutoscaler", "cdi-deployment"),
	}
}

func cdiCRDResources() []staleResource {
	return []staleResource{
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "cdiconfigs.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "datavolumes.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "dataimportcrons.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "datasources.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "objecttransfers.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "openstackvolumepopulators.forklift.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "ovirtvolumepopulators.forklift.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "volumeclonesources.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "volumeimportsources.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "volumeuploadsources.cdi.kubevirt.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationdatavolumes.cdi.internal.virtualization.deckhouse.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationdataimportcrons.cdi.internal.virtualization.deckhouse.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationdatasources.cdi.internal.virtualization.deckhouse.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationobjecttransfers.cdi.internal.virtualization.deckhouse.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationvolumeclonesources.cdi.internal.virtualization.deckhouse.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationvolumeimportsources.cdi.internal.virtualization.deckhouse.io"),
		cluster("apiextensions.k8s.io/v1", "CustomResourceDefinition", "internalvirtualizationvolumeuploadsources.cdi.internal.virtualization.deckhouse.io"),
	}
}

func namespaced(apiVersion, kind, name string) staleResource {
	return staleResource{apiVersion: apiVersion, kind: kind, namespace: cdiCleanupNamespace, name: name}
}

func cluster(apiVersion, kind, name string) staleResource {
	return staleResource{apiVersion: apiVersion, kind: kind, name: name}
}
