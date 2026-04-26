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

package precheck

import (
	"context"
	"fmt"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	storageClassPrecheckEnvName = "STORAGECLASS_PRECHECK"
)

// storageClassPrecheck implements Precheck interface for default StorageClass.
// This is a common precheck that runs for all tests.
type storageClassPrecheck struct{}

func (c *storageClassPrecheck) Label() string {
	return PrecheckStorageClass
}

func (c *storageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(storageClassPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("StorageClass precheck is disabled.\n"))
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	err := k8sClient.List(ctx, &scList)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", storageClassPrecheckEnvName, err)
	}

	var defaultClasses []storagev1.StorageClass
	for i := range scList.Items {
		sc := &scList.Items[i]
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultClasses = append(defaultClasses, *sc)
		}
	}

	if len(defaultClasses) == 0 {
		return fmt.Errorf("%s=no to disable this precheck: cluster has no default StorageClass. "+
			"Please set a default StorageClass with: kubectl annotate storageclass/<name> storageclass.kubernetes.io/is-default-class=true",
			storageClassPrecheckEnvName)
	}

	// Sort by creation timestamp, newest first
	// Secondary sort by name, ascending order
	sort.Slice(defaultClasses, func(i, j int) bool {
		if defaultClasses[i].CreationTimestamp.UnixNano() == defaultClasses[j].CreationTimestamp.UnixNano() {
			return defaultClasses[i].Name < defaultClasses[j].Name
		}
		return defaultClasses[i].CreationTimestamp.UnixNano() > defaultClasses[j].CreationTimestamp.UnixNano()
	})

	return nil
}

// Register StorageClass precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&storageClassPrecheck{}, true)
}
