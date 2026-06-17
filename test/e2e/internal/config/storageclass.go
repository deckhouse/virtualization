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

// FindImmediateStorageClass finds an immediate StorageClass with the same provisioner as defaultSC.
// It checks if defaultSC has Immediate binding mode first, then searches for an immediate SC with same provisioner.
// Returns the immediate StorageClass if found, or nil if not found.
func FindImmediateStorageClass(defaultSC *storagev1.StorageClass, scList *storagev1.StorageClassList) *storagev1.StorageClass {
	if defaultSC == nil {
		return nil
	}

	// If default StorageClass already has Immediate binding mode, use it
	if defaultSC.VolumeBindingMode != nil &&
		*defaultSC.VolumeBindingMode == storagev1.VolumeBindingImmediate {
		return defaultSC
	}

	// Find immediate StorageClass with same provisioner
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.VolumeBindingMode == nil {
			continue
		}
		if *sc.VolumeBindingMode == storagev1.VolumeBindingImmediate &&
			sc.Provisioner == defaultSC.Provisioner {
			return sc
		}
	}

	return nil
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

	c.StorageClass.ImmediateStorageClass = FindImmediateStorageClass(c.StorageClass.DefaultStorageClass, &scList)

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
