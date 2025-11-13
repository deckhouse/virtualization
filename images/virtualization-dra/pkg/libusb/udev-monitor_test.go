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

package libusb

import (
	"context"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-dra/pkg/logger"
	"github.com/deckhouse/virtualization-dra/pkg/udev"
)

func newFakeUdevMonitor() *fakeUdevMonitor {
	return &fakeUdevMonitor{
		eventCh: make(chan *udev.UEvent, 100),
		errorCh: make(chan error),
	}
}

type fakeUdevMonitor struct {
	eventCh chan *udev.UEvent
	errorCh chan error
}

func (f *fakeUdevMonitor) pushEvent(uevent *udev.UEvent) {
	f.eventCh <- uevent
}

func (f *fakeUdevMonitor) Start(_ context.Context) (<-chan *udev.UEvent, <-chan error) {
	return f.eventCh, f.errorCh
}

var _ = Describe("Udev monitor", func() {
	var (
		fakeMonitor *fakeUdevMonitor
		usbMonitor  *UdevMonitor
	)

	newUSBMonitor := func() *UdevMonitor {
		log := slog.New(logger.NewLogger("", "discard", 0).Handler())
		m := &UdevMonitor{
			store:            NewUSBDeviceStore(nil, log),
			udevMonitor:      fakeMonitor,
			log:              log,
			resyncPeriod:     5 * time.Minute,
			debounceDuration: 100 * time.Millisecond,
			pendingEvents:    make(map[string]*debounceEntry),
		}
		return m
	}

	BeforeEach(func() {
		pathToUSBDevices = "testdata/sys/bus/usb/devices"
		fakeMonitor = newFakeUdevMonitor()
		usbMonitor = newUSBMonitor()
	})

	It("should monitor usb devices", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go usbMonitor.run(ctx)

		devices := usbMonitor.GetDevices()
		Expect(devices).To(HaveLen(0))

		fakeMonitor.pushEvent(&udev.UEvent{
			Action: udev.ActionAdd,
			KObj:   testUsb.Path,
		})

		Eventually(func(g Gomega) {
			devices = usbMonitor.GetDevices()
			g.Expect(devices).To(HaveLen(1))
			compareUsb(&devices[0], &testUsb)

			device, found := usbMonitor.GetDeviceByBusID(testUsb.BusID)
			Expect(found).To(BeTrue())
			compareUsb(device, &testUsb)
		}).WithPolling(time.Second).WithTimeout(10 * time.Second).Should(Succeed())

		fakeMonitor.pushEvent(&udev.UEvent{
			Action: udev.ActionRemove,
			KObj:   testUsb.Path,
		})

		Eventually(func(g Gomega) {
			devices = usbMonitor.GetDevices()
			g.Expect(devices).To(HaveLen(0))

			device, found := usbMonitor.GetDeviceByBusID(testUsb.BusID)
			Expect(found).To(BeFalse())
			Expect(device).To(BeNil())
		}).WithPolling(time.Second).WithTimeout(10 * time.Second).Should(Succeed())
	})
})
