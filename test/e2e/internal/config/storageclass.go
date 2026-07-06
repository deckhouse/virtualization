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

	// WFFCStorageClassEnv overrides the WaitForFirstConsumer StorageClass used by block-device
	// tests. When unset, the class is derived from the default StorageClass: the default itself
	// when it uses WaitForFirstConsumer, or another StorageClass on the same CSI driver when
	// the default uses Immediate binding.
	WFFCStorageClassEnv = "WFFC_STORAGE_CLASS"

	// ImmediateStorageClassEnv overrides the Immediate StorageClass used by block-device tests.
	// When unset, the class is derived from the default StorageClass: the default itself when
	// it uses Immediate binding, or another StorageClass on the same CSI driver when the
	// default uses WaitForFirstConsumer.
	ImmediateStorageClassEnv = "IMMEDIATE_STORAGE_CLASS"
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

// IsWFFCBinding reports whether sc uses the WaitForFirstConsumer volume binding mode.
func IsWFFCBinding(sc *storagev1.StorageClass) bool {
	return sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
}

// IsImmediateBinding reports whether sc uses the Immediate volume binding mode. A nil
// VolumeBindingMode defaults to Immediate per the Kubernetes API.
func IsImmediateBinding(sc *storagev1.StorageClass) bool {
	return sc.VolumeBindingMode == nil || *sc.VolumeBindingMode == storagev1.VolumeBindingImmediate
}

// VolumeBindingMode returns sc's volume binding mode for diagnostics, rendering a nil
// mode as its Immediate default.
func VolumeBindingMode(sc *storagev1.StorageClass) storagev1.VolumeBindingMode {
	if sc.VolumeBindingMode == nil {
		return storagev1.VolumeBindingImmediate
	}
	return *sc.VolumeBindingMode
}

// FindStorageClassWithDifferentProvisioner returns a StorageClass whose provisioner is a
// registered CSI driver different from the given provisioner. Non-CSI provisioners (for
// example the localpath external provisioner) are skipped: virtualization does not work
// with them. When several match, the one with the smallest name is returned for
// determinism. Returns nil if none match (the cluster has only a single CSI driver).
func FindStorageClassWithDifferentProvisioner(scList *storagev1.StorageClassList, csiDrivers *storagev1.CSIDriverList, provisioner string) *storagev1.StorageClass {
	registered := make(map[string]struct{}, len(csiDrivers.Items))
	for _, driver := range csiDrivers.Items {
		registered[driver.Name] = struct{}{}
	}

	var match *storagev1.StorageClass
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.Provisioner == "" || sc.Provisioner == provisioner {
			continue
		}
		if _, ok := registered[sc.Provisioner]; !ok {
			continue
		}
		if match == nil || sc.Name < match.Name {
			match = sc
		}
	}

	return match
}

// FindStorageClassWithProvisionerAndBinding returns a StorageClass backed by provisioner
// with the requested volume binding mode. When several match, the most recently created
// one is returned (ties are broken by name, ascending).
func FindStorageClassWithProvisionerAndBinding(
	scList *storagev1.StorageClassList,
	provisioner string,
	wffc bool,
) *storagev1.StorageClass {
	var matched []storagev1.StorageClass
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.Provisioner != provisioner {
			continue
		}
		if wffc && IsWFFCBinding(sc) {
			matched = append(matched, *sc)
		}
		if !wffc && IsImmediateBinding(sc) {
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

// ResolveWFFCStorageClass returns the WaitForFirstConsumer StorageClass for block-device
// tests. When WFFC_STORAGE_CLASS is set, that StorageClass is returned. Otherwise the class
// is derived from the default StorageClass: the default itself when it uses
// WaitForFirstConsumer, or another StorageClass on the same CSI driver when the default
// uses Immediate binding. Returns nil when no default StorageClass is configured and the
// env var is unset, or when auto-detection finds no matching StorageClass.
func ResolveWFFCStorageClass(scList *storagev1.StorageClassList) (*storagev1.StorageClass, error) {
	if sc, err := resolveStorageClassFromEnv(scList, WFFCStorageClassEnv); err != nil || sc != nil {
		return sc, err
	}

	defaultSC := FindDefaultStorageClass(scList)
	if defaultSC == nil {
		return nil, nil
	}

	if IsWFFCBinding(defaultSC) {
		return defaultSC, nil
	}

	if IsImmediateBinding(defaultSC) {
		return FindStorageClassWithProvisionerAndBinding(scList, defaultSC.Provisioner, true), nil
	}

	return nil, nil
}

// ResolveImmediateStorageClass returns the Immediate StorageClass for block-device tests.
// When IMMEDIATE_STORAGE_CLASS is set, that StorageClass is returned. Otherwise the class
// is derived from the default StorageClass: the default itself when it uses Immediate
// binding, or another StorageClass on the same CSI driver when the default uses
// WaitForFirstConsumer. Returns nil when no default StorageClass is configured and the
// env var is unset, or when auto-detection finds no matching StorageClass.
func ResolveImmediateStorageClass(scList *storagev1.StorageClassList) (*storagev1.StorageClass, error) {
	if sc, err := resolveStorageClassFromEnv(scList, ImmediateStorageClassEnv); err != nil || sc != nil {
		return sc, err
	}

	defaultSC := FindDefaultStorageClass(scList)
	if defaultSC == nil {
		return nil, nil
	}

	if IsImmediateBinding(defaultSC) {
		return defaultSC, nil
	}

	if IsWFFCBinding(defaultSC) {
		return FindStorageClassWithProvisionerAndBinding(scList, defaultSC.Provisioner, false), nil
	}

	return nil, nil
}

// ResolveTemplateStorageClass returns the StorageClass used by tests that allow either an
// explicit STORAGE_CLASS_NAME override or the cluster default StorageClass.
func ResolveTemplateStorageClass(scList *storagev1.StorageClassList) (*storagev1.StorageClass, error) {
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
// TemplateStorageClass is taken from StorageClassNameEnv when set, otherwise DefaultStorageClass is used.
func (c *Config) SetStorageClasses(ctx context.Context, k8sClient client.Client) error {
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("failed to list StorageClasses: %w", err)
	}

	c.StorageClass.DefaultStorageClass = FindDefaultStorageClass(&scList)

	wffcSC, err := ResolveWFFCStorageClass(&scList)
	if err != nil {
		return err
	}
	c.StorageClass.WFFCStorageClass = wffcSC

	immediateSC, err := ResolveImmediateStorageClass(&scList)
	if err != nil {
		return err
	}
	c.StorageClass.ImmediateStorageClass = immediateSC

	if c.StorageClass.WFFCStorageClass != nil {
		var csiDrivers storagev1.CSIDriverList
		if err := k8sClient.List(ctx, &csiDrivers); err != nil {
			return fmt.Errorf("failed to list CSIDrivers: %w", err)
		}
		c.StorageClass.DifferentCSIDriverStorageClass = FindStorageClassWithDifferentProvisioner(
			&scList, &csiDrivers, c.StorageClass.WFFCStorageClass.Provisioner,
		)
	}

	templateSC, err := ResolveTemplateStorageClass(&scList)
	if err != nil {
		return err
	}
	if templateSC != nil {
		c.StorageClass.TemplateStorageClass = templateSC
	}

	return nil
}

func resolveStorageClassFromEnv(scList *storagev1.StorageClassList, envName string) (*storagev1.StorageClass, error) {
	scName, ok := os.LookupEnv(envName)
	if !ok || scName == "" {
		return nil, nil
	}

	if sc := findStorageClassInList(scList, scName); sc != nil {
		return sc, nil
	}

	return nil, fmt.Errorf("StorageClass %q from %s env not found", scName, envName)
}

func findStorageClassInList(scList *storagev1.StorageClassList, name string) *storagev1.StorageClass {
	for i := range scList.Items {
		if scList.Items[i].Name == name {
			return &scList.Items[i]
		}
	}
	return nil
}
