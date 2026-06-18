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

package config

import (
	"context"
	"fmt"
	"os"
	"sort"

	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NFS = "nfs.csi.k8s.io"

	// StorageClassNameEnv overrides TemplateStorageClass for tests (see README).
	StorageClassNameEnv = "STORAGE_CLASS_NAME"

	// WFFCStorageClassAnnotation marks the StorageClass that e2e block-device tests use to
	// provision the scenario's main VirtualDisks and VirtualImages. It must use the
	// WaitForFirstConsumer volume binding mode.
	WFFCStorageClassAnnotation = "e2e.virtualization.deckhouse.io/is-wffc-storage-class"

	// ImmediateStorageClassAnnotation marks the StorageClass that e2e block-device tests use
	// as the "other" StorageClass when a source object must live on a different StorageClass
	// than the produced one, and to provision dependent objects that must become Ready without
	// a consumer. It must be backed by the same CSI driver as the WFFC one and use the
	// Immediate volume binding mode.
	ImmediateStorageClassAnnotation = "e2e.virtualization.deckhouse.io/is-immediate-storage-class"
)

// FindDefaultStorageClass returns the default StorageClass from the list.
// It selects the most recently created default StorageClass (by creationTimestamp).
// If there are multiple with the same timestamp, sorts by name ascending.
// Returns nil if no default StorageClass is found.
func FindDefaultStorageClass(scList *storagev1.StorageClassList) *storagev1.StorageClass {
	var defaultClasses []storagev1.StorageClass
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultClasses = append(defaultClasses, *sc)
		}
	}

	if len(defaultClasses) == 0 {
		return nil
	}

	// Sort by creation timestamp, newest first
	// Secondary sort by name, ascending order
	sort.Slice(defaultClasses, func(i, j int) bool {
		if defaultClasses[i].CreationTimestamp.UnixNano() == defaultClasses[j].CreationTimestamp.UnixNano() {
			return defaultClasses[i].Name < defaultClasses[j].Name
		}
		return defaultClasses[i].CreationTimestamp.UnixNano() > defaultClasses[j].CreationTimestamp.UnixNano()
	})

	return &defaultClasses[0]
}

// FindStorageClassByAnnotation returns the StorageClass whose annotation `annotation`
// is set to "true". When several match, the most recently created one is returned
// (ties are broken by name, ascending). Returns nil if none match.
func FindStorageClassByAnnotation(scList *storagev1.StorageClassList, annotation string) *storagev1.StorageClass {
	var matched []storagev1.StorageClass
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.Annotations[annotation] == "true" {
			matched = append(matched, *sc)
		}
	}

	if len(matched) == 0 {
		return nil
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreationTimestamp.UnixNano() == matched[j].CreationTimestamp.UnixNano() {
			return matched[i].Name < matched[j].Name
		}
		return matched[i].CreationTimestamp.UnixNano() > matched[j].CreationTimestamp.UnixNano()
	})

	return &matched[0]
}

// FindStorageClassWithDifferentProvisioner returns a StorageClass whose provisioner (CSI
// driver) differs from the given provisioner. When several match, the one with the
// smallest name is returned for determinism. Returns nil if none match (the cluster has
// only a single CSI driver).
func FindStorageClassWithDifferentProvisioner(scList *storagev1.StorageClassList, provisioner string) *storagev1.StorageClass {
	var match *storagev1.StorageClass
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.Provisioner == "" || sc.Provisioner == provisioner {
			continue
		}
		if match == nil || sc.Name < match.Name {
			match = sc
		}
	}

	return match
}

// SetStorageClasses discovers cluster StorageClasses and populates Config.StorageClass fields.
// TemplateStorageClass is taken from StorageClassNameEnv when set, otherwise DefaultStorageClass is used.
func (c *Config) SetStorageClasses(ctx context.Context, k8sClient client.Client) error {
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("failed to list StorageClasses: %w", err)
	}

	c.StorageClass.DefaultStorageClass = FindDefaultStorageClass(&scList)
	if c.StorageClass.DefaultStorageClass == nil {
		return fmt.Errorf("default StorageClass not found in the cluster")
	}

	// Discovered non-fatally; their presence (and the volume binding modes of the WFFC and
	// immediate StorageClasses) is enforced by the dedicated prechecks for tests that require them.
	c.StorageClass.WFFCStorageClass = FindStorageClassByAnnotation(&scList, WFFCStorageClassAnnotation)
	c.StorageClass.ImmediateStorageClass = FindStorageClassByAnnotation(&scList, ImmediateStorageClassAnnotation)
	// The different-CSI-driver StorageClass is discovered automatically: any StorageClass
	// whose CSI driver differs from the WFFC one. No annotation is required.
	if c.StorageClass.WFFCStorageClass != nil {
		c.StorageClass.DifferentCSIDriverStorageClass = FindStorageClassWithDifferentProvisioner(&scList, c.StorageClass.WFFCStorageClass.Provisioner)
	}

	templateSC, err := findStorageClassFromEnv(ctx, k8sClient, StorageClassNameEnv, &scList)
	if err != nil {
		return err
	}
	if templateSC != nil {
		c.StorageClass.TemplateStorageClass = templateSC
	} else {
		c.StorageClass.TemplateStorageClass = c.StorageClass.DefaultStorageClass
	}

	return nil
}

func findStorageClassFromEnv(
	ctx context.Context,
	k8sClient client.Client,
	envName string,
	scList *storagev1.StorageClassList,
) (*storagev1.StorageClass, error) {
	scName, ok := os.LookupEnv(envName)
	if !ok {
		return nil, nil
	}

	for i := range scList.Items {
		if scList.Items[i].Name == scName {
			return &scList.Items[i], nil
		}
	}

	sc := &storagev1.StorageClass{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		return nil, fmt.Errorf("failed to get StorageClass %q from %s env: %w", scName, envName, err)
	}

	return sc, nil
}
