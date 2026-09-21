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

package usb

import (
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

type fakeBinder struct {
	bound    map[string]bool
	unbound  []string
	checkErr error
}

func (f *fakeBinder) Bind(busID string) error { f.bound[busID] = true; return nil }
func (f *fakeBinder) Unbind(busID string) error {
	f.unbound = append(f.unbound, busID)
	f.bound[busID] = false
	return nil
}
func (f *fakeBinder) IsBound(busID string) (bool, error)     { return f.bound[busID], f.checkErr }
func (f *fakeBinder) GetBindInfo() ([]usbip.BindInfo, error) { return nil, nil }

var _ = Describe("AllocationStore reclaimExportedDevice", func() {
	var (
		binder *fakeBinder
		store  *AllocationStore
	)

	BeforeEach(func() {
		binder = &fakeBinder{bound: map[string]bool{}}
		store = &AllocationStore{usbBinder: binder, log: slog.Default()}
	})

	It("leaves a device that is not exported alone", func() {
		Expect(store.reclaimExportedDevice("usb-a", "8-1")).To(Succeed())
		Expect(binder.unbound).To(BeEmpty())
	})

	It("unbinds a device exported to another node", func() {
		binder.bound["8-1"] = true
		Expect(store.reclaimExportedDevice("usb-a", "8-1")).To(Succeed())
		Expect(binder.unbound).To(Equal([]string{"8-1"}))
	})

	It("fails when the export state cannot be read", func() {
		binder.checkErr = errors.New("sysfs is gone")
		Expect(store.reclaimExportedDevice("usb-a", "8-1")).To(MatchError(ContainSubstring("sysfs is gone")))
	})
})
