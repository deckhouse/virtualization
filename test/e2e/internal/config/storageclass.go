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

	// StorageClassNameEnv overrides DefaultStorageClass for tests (see README).
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

// ResolveDefaultStorageClass returns the StorageClass for the suite: an explicit
// STORAGE_CLASS_NAME override when set, otherwise the cluster default StorageClass.
func ResolveDefaultStorageClass(scList *storagev1.StorageClassList) (*storagev1.StorageClass, error) {
	scName, ok := os.LookupEnv(StorageClassNameEnv)
	if ok {
		if scName == "" {
			return nil, fmt.Errorf("%s env is set but empty", StorageClassNameEnv)
		}
		if sc := findStorageClassInList(scList, scName); sc != nil {
			return sc, nil
		}
		return nil, fmt.Errorf("StorageClass %q from %s env not found", scName, StorageClassNameEnv)
	}

	return FindDefaultStorageClass(scList), nil
}

// SetStorageClasses discovers cluster StorageClasses and populates Config.StorageClass fields.
// DefaultStorageClass is taken from StorageClassNameEnv when set, otherwise the cluster default
// StorageClass is used.
func (c *Config) SetStorageClasses(ctx context.Context, k8sClient client.Client) error {
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("failed to list StorageClasses: %w", err)
	}

	defaultSC, err := ResolveDefaultStorageClass(&scList)
	if err != nil {
		return err
	}
	c.StorageClass.DefaultStorageClass = defaultSC

	return nil
}

func findStorageClassInList(scList *storagev1.StorageClassList, name string) *storagev1.StorageClass {
	for i := range scList.Items {
		if scList.Items[i].Name == name {
			return &scList.Items[i]
		}
	}
	return nil
}
