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

package blockdevice

import (
	"context"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// IsNFS returns true if the storage class is NFS.
func IsNFS() bool {
	sc := framework.GetConfig().StorageClass.TemplateStorageClass
	if sc == nil {
		return false
	}
	return sc.Provisioner == framework.NFS
}

// needPublishOption returns true if publish option should be used for export.
func needPublishOption(f *framework.Framework) bool {
	hostname, err := os.Hostname()
	Expect(err).NotTo(HaveOccurred(), "Failed to get hostname")
	var node corev1.Node
	err = f.Clients.GenericClient().Get(
		context.Background(),
		types.NamespacedName{Name: hostname},
		&node,
	)
	if k8serrors.IsNotFound(err) {
		return true
	}
	Expect(err).NotTo(HaveOccurred(), "Failed to get node %s", hostname)
	return false
}

// DataExport exports a resource (VirtualDisk, VirtualDiskSnapshot, etc.) to a local file.
// Automatically cleans up the exported file after test.
//
// resourceType: "vd" for VirtualDisk, "vds" for VirtualDiskSnapshot, "vi" for VirtualImage, etc.
// Use needPublishOption and IsNFS from data_exports.go for configuration.
func DataExport(f *framework.Framework, resourceType, name, outputFile string) {
	opts := d8.DataExportOptions{
		Namespace:  f.Namespace().Name,
		OutputFile: outputFile,
		Publish:    needPublishOption(f),
		Timeout:    framework.LongTimeout,
		Cleanup:    true,
	}
	if IsNFS() {
		opts.SourcePath = diskImageExportFile
	}
	err := f.D8Virtualization().DataExportDownload(resourceType, name, opts)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err := os.Remove(outputFile)
		Expect(err == nil || errors.Is(err, os.ErrNotExist)).To(BeTrue(),
			"Failed to remove exported file %s: %v", outputFile, err)
	})
}
