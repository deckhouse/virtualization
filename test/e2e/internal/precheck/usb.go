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

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	usbPrecheckEnvName = "USB_PRECHECK"

	// dummyHCDVendorID is the VendorID for dummy_hcd USB device.
	dummyHCDVendorID = "1d6b"
	// dummyHCDProductID is the ProductID for dummy_hcd USB device.
	dummyHCDProductID = "0104"
)

// usbPrecheck implements Precheck interface for USB dummy_hcd.
type usbPrecheck struct{}

func (u *usbPrecheck) Label() string {
	return PrecheckUSB
}

// checkDummyHCDConfigured checks if dummy_hcd USB device is configured.
// dummy_hcd is a virtual USB device used for testing USB passthrough.
func checkDummyHCDConfigured(ctx context.Context, f *framework.Framework) bool {
	virtClient := f.VirtClient()

	nodeUSBList, err := virtClient.NodeUSBDevices().List(ctx, metav1.ListOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "failed to list NodeUSBDevices: %v\n", err)
		return false
	}

	for _, nodeUSB := range nodeUSBList.Items {
		if nodeUSB.Status.Attributes.VendorID == dummyHCDVendorID && nodeUSB.Status.Attributes.ProductID == dummyHCDProductID {
			return true
		}
	}

	return false
}

func (u *usbPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(usbPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("USB precheck is disabled.\n"))
		return nil
	}

	if !checkDummyHCDConfigured(ctx, f) {
		return fmt.Errorf("%s=no to disable this precheck: dummy_hcd USB device is not configured. "+
			"Run generate_dummy_hcd_ngc.sh to configure dummy_hcd USB device", usbPrecheckEnvName)
	}

	return nil
}

// Register USB precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&usbPrecheck{}, false)
}
