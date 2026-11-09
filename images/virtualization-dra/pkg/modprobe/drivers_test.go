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

package modprobe

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DriversRegistered", func() {
	drivers := []string{"bus/usb/drivers/usbip-host", "bus/platform/drivers/vhci_hcd"}

	DescribeTable("reports whether every driver is registered",
		func(present, check []string, want bool) {
			sysRoot := GinkgoT().TempDir()
			for _, driver := range present {
				Expect(os.MkdirAll(filepath.Join(sysRoot, driver), 0o755)).To(Succeed())
			}

			Expect(DriversRegistered(sysRoot, check...)).To(Equal(want))
		},
		Entry("all drivers registered", drivers, drivers, true),
		Entry("no drivers registered", nil, drivers, false),
		Entry("one driver missing", drivers[:1], drivers, false),
		Entry("nothing to check", drivers, nil, false),
	)
})
