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

package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// IsSvdmModuleEnabled checks if the SVDM (Storage Volume Data Manager) module is enabled.
func IsSvdmModuleEnabled(f *framework.Framework) bool {
	GinkgoHelper()

	svdmModule, err := f.GetModuleConfig("storage-volume-data-manager")
	Expect(err).NotTo(HaveOccurred())
	enabled := svdmModule.Spec.Enabled

	return enabled != nil && *enabled
}