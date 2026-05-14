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

package util

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// GetDefaultStorageClass loads cluster StorageClasses and returns the current default one.
func GetDefaultStorageClass(ctx context.Context, f *framework.Framework) (*storagev1.StorageClass, *storagev1.StorageClassList) {
	GinkgoHelper()

	scList := &storagev1.StorageClassList{}
	err := f.GenericClient().List(ctx, scList)
	Expect(err).NotTo(HaveOccurred())

	defaultSC := config.FindDefaultStorageClass(scList)
	Expect(defaultSC).NotTo(BeNil(), "default storage class cannot be nil")

	return defaultSC, scList
}
