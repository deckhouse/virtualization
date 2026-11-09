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
)

// DriversRegistered reports whether every driver directory exists under sysRoot,
// e.g. "bus/usb/drivers/usbip-host". A driver is registered there both when it is
// a loaded module and when it is built into the kernel, which has no .ko file to load.
func DriversRegistered(sysRoot string, drivers ...string) bool {
	if len(drivers) == 0 {
		return false
	}

	for _, driver := range drivers {
		if _, err := os.Stat(filepath.Join(sysRoot, driver)); err != nil {
			return false
		}
	}

	return true
}
