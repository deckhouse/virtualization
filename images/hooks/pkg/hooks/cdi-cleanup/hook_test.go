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
	"strings"
	"testing"
)

func TestStaleCDIResources(t *testing.T) {
	resources := staleCDIResources()

	assertHasResource(t, resources, staleResource{
		apiVersion: "apps/v1",
		kind:       "Deployment",
		namespace:  cdiCleanupNamespace,
		name:       "cdi-apiserver",
	})
	assertHasResource(t, resources, staleResource{
		apiVersion: "apps/v1",
		kind:       "Deployment",
		namespace:  cdiCleanupNamespace,
		name:       "cdi-deployment",
	})
	assertHasResource(t, resources, staleResource{
		apiVersion: "apps/v1",
		kind:       "Deployment",
		namespace:  cdiCleanupNamespace,
		name:       "cdi-operator",
	})
	assertHasResource(t, resources, staleResource{
		apiVersion: "apiextensions.k8s.io/v1",
		kind:       "CustomResourceDefinition",
		name:       "datavolumes.cdi.kubevirt.io",
	})

	for _, resource := range resources {
		if strings.Contains(resource.name, "storageprofiles") {
			t.Fatalf("StorageProfile CRD must not be removed by CDI cleanup hook: %#v", resource)
		}
		if resource.kind == "InternalVirtualizationCDI" || resource.name == "config" && resource.apiVersion == "cdi.internal.virtualization.deckhouse.io/v1beta1" {
			t.Fatalf("CDI config must not be removed by CDI cleanup hook: %#v", resource)
		}
	}
}

func assertHasResource(t *testing.T, resources []staleResource, want staleResource) {
	t.Helper()

	for _, resource := range resources {
		if resource == want {
			return
		}
	}

	t.Fatalf("expected stale CDI resource not found: %#v", want)
}
