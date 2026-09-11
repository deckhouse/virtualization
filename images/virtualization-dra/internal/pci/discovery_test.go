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

package pci

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

type fakeBlockDevice struct {
	name      string
	hasHolder bool
	mounted   bool
	swap      bool
}

type fakeDevice struct {
	address    string
	vendor     string
	device     string
	class      string
	iommuGroup int
	driver     string
	netUp      *bool
	blocks     []fakeBlockDevice
}

type fakeSysfs struct {
	root string
	proc string
}

func newFakeSysfs() *fakeSysfs {
	f := &fakeSysfs{root: GinkgoT().TempDir(), proc: GinkgoT().TempDir()}
	Expect(os.WriteFile(filepath.Join(f.proc, "mounts"), nil, 0o644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(f.proc, "swaps"), []byte("Filename Type Size Used Priority\n"), 0o644)).To(Succeed())
	return f
}

func (f *fakeSysfs) appendFile(path, line string) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	Expect(err).ToNot(HaveOccurred())
	defer file.Close()
	_, err = file.WriteString(line)
	Expect(err).ToNot(HaveOccurred())
}

func (f *fakeSysfs) add(dev fakeDevice) {
	deviceDir := filepath.Join(f.root, "bus", "pci", "devices", dev.address)
	Expect(os.MkdirAll(deviceDir, 0o755)).To(Succeed())

	writeFile := func(name, value string) {
		Expect(os.WriteFile(filepath.Join(deviceDir, name), []byte(value+"\n"), 0o644)).To(Succeed())
	}
	writeFile("vendor", "0x"+dev.vendor)
	writeFile("device", "0x"+dev.device)
	writeFile("class", "0x"+dev.class)
	writeFile("numa_node", "0")

	if dev.iommuGroup >= 0 {
		groupDir := filepath.Join(f.root, "kernel", "iommu_groups", strconv.Itoa(dev.iommuGroup))
		groupDevicesDir := filepath.Join(groupDir, "devices")
		Expect(os.MkdirAll(groupDevicesDir, 0o755)).To(Succeed())
		Expect(os.Symlink(groupDir, filepath.Join(deviceDir, "iommu_group"))).To(Succeed())
		Expect(os.Symlink(deviceDir, filepath.Join(groupDevicesDir, dev.address))).To(Succeed())
	}

	if dev.driver != "" {
		driverDir := filepath.Join(f.root, "bus", "pci", "drivers", dev.driver)
		Expect(os.MkdirAll(driverDir, 0o755)).To(Succeed())
		Expect(os.Symlink(driverDir, filepath.Join(deviceDir, "driver"))).To(Succeed())
	}

	if dev.netUp != nil {
		netDir := filepath.Join(deviceDir, "virtio0", "net", "eth-"+dev.address)
		Expect(os.MkdirAll(netDir, 0o755)).To(Succeed())
		classNetDir := filepath.Join(f.root, "class", "net")
		Expect(os.MkdirAll(classNetDir, 0o755)).To(Succeed())
		Expect(os.Symlink(netDir, filepath.Join(classNetDir, "eth-"+dev.address))).To(Succeed())
		flags := "0x1002"
		if *dev.netUp {
			flags = "0x1003"
		}
		Expect(os.WriteFile(filepath.Join(netDir, "flags"), []byte(flags+"\n"), 0o644)).To(Succeed())
	}

	for _, block := range dev.blocks {
		realDir := filepath.Join(deviceDir, "nvme", "nvme0", block.name)
		Expect(os.MkdirAll(filepath.Join(realDir, "holders"), 0o755)).To(Succeed())
		classBlockDir := filepath.Join(f.root, "class", "block")
		Expect(os.MkdirAll(classBlockDir, 0o755)).To(Succeed())
		Expect(os.Symlink(realDir, filepath.Join(classBlockDir, block.name))).To(Succeed())
		if block.hasHolder {
			Expect(os.WriteFile(filepath.Join(realDir, "holders", "dm-0"), nil, 0o644)).To(Succeed())
		}
		if block.mounted {
			f.appendFile(filepath.Join(f.proc, "mounts"), "/dev/"+block.name+" /mnt ext4 rw 0 0\n")
		}
		if block.swap {
			f.appendFile(filepath.Join(f.proc, "swaps"), "/dev/"+block.name+" partition 1024 0 -2\n")
		}
	}
}

func (f *fakeSysfs) discover() map[string]Device {
	fs := newSysfs(f.root)
	fs.proc = f.proc
	devices, err := fs.discoverDevices(slog.Default())
	Expect(err).ToNot(HaveOccurred())
	return devices
}

func boolPtr(v bool) *bool { return &v }

var _ = Describe("PCI device discovery", func() {
	It("discovers an eligible endpoint device", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:3b:00.0", vendor: "10ee", device: "9038", class: "120000", iommuGroup: 42, driver: "xclmgmt"})

		devices := fs.discover()
		Expect(devices).To(HaveKey("0000:3b:00.0"))
		Expect(devices["0000:3b:00.0"]).To(Equal(Device{
			Address:    "0000:3b:00.0",
			VendorID:   "10ee",
			DeviceID:   "9038",
			ClassCode:  "120000",
			Driver:     "xclmgmt",
			IOMMUGroup: 42,
			NUMANode:   0,
		}))
	})

	DescribeTable("excludes host-critical device classes",
		func(class string) {
			fs := newFakeSysfs()
			fs.add(fakeDevice{address: "0000:01:00.0", vendor: "10de", device: "1eb8", class: class, iommuGroup: 1})

			Expect(fs.discover()).To(BeEmpty())
		},
		Entry("display controller", "030000"),
		Entry("memory controller", "058000"),
		Entry("bridge", "060400"),
		Entry("system peripheral", "088000"),
	)

	DescribeTable("storage controllers follow the block device occupancy",
		func(blocks []fakeBlockDevice, published bool) {
			fs := newFakeSysfs()
			fs.add(fakeDevice{address: "0000:04:00.0", vendor: "144d", device: "a80a", class: "010802", iommuGroup: 4, blocks: blocks})

			devices := fs.discover()
			if published {
				Expect(devices).To(HaveKey("0000:04:00.0"))
			} else {
				Expect(devices).To(BeEmpty())
			}
		},
		Entry("free NVMe without block devices is published", nil, true),
		Entry("free NVMe with an unused namespace is published",
			[]fakeBlockDevice{{name: "nvme0n1"}}, true),
		Entry("NVMe with a mounted namespace is excluded",
			[]fakeBlockDevice{{name: "nvme0n1"}, {name: "nvme0n1p1", mounted: true}}, false),
		Entry("NVMe stacked under device-mapper is excluded",
			[]fakeBlockDevice{{name: "nvme0n1", hasHolder: true}}, false),
		Entry("NVMe used as swap is excluded",
			[]fakeBlockDevice{{name: "nvme0n1", swap: true}}, false),
	)

	It("excludes a root-complex-integrated endpoint on bus 00", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:00:0a.0", vendor: "8086", device: "467d", class: "118000", iommuGroup: 4})

		Expect(fs.discover()).To(BeEmpty())
	})

	It("excludes a device without an IOMMU group", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:01:00.0", vendor: "10ee", device: "9038", class: "120000", iommuGroup: -1})

		Expect(fs.discover()).To(BeEmpty())
	})

	It("excludes a network device with an active interface", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:02:00.0", vendor: "8086", device: "1533", class: "020000", iommuGroup: 2, netUp: boolPtr(true)})

		Expect(fs.discover()).To(BeEmpty())
	})

	It("discovers a network device whose interfaces are down", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:02:00.0", vendor: "8086", device: "1533", class: "020000", iommuGroup: 2, netUp: boolPtr(false)})

		Expect(fs.discover()).To(HaveKey("0000:02:00.0"))
	})

	It("excludes a device sharing an IOMMU group with a denied device", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:03:00.0", vendor: "10de", device: "1eb8", class: "030000", iommuGroup: 3})
		fs.add(fakeDevice{address: "0000:03:00.1", vendor: "10de", device: "10f8", class: "040300", iommuGroup: 3})

		Expect(fs.discover()).To(BeEmpty())
	})

	It("discovers a device sharing an IOMMU group only with a bridge", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:00:1c.0", vendor: "8086", device: "a33c", class: "060400", iommuGroup: 4})
		fs.add(fakeDevice{address: "0000:04:00.0", vendor: "1912", device: "0014", class: "0c0330", iommuGroup: 4})

		devices := fs.discover()
		Expect(devices).To(HaveLen(1))
		Expect(devices).To(HaveKey("0000:04:00.0"))
	})

	It("discovers all devices of a clean multi-function group", func() {
		fs := newFakeSysfs()
		fs.add(fakeDevice{address: "0000:05:00.0", vendor: "10ee", device: "9038", class: "120000", iommuGroup: 5})
		fs.add(fakeDevice{address: "0000:05:00.1", vendor: "10ee", device: "9039", class: "120000", iommuGroup: 5})

		Expect(fs.discover()).To(HaveLen(2))
	})
})

var _ = Describe("Device name", func() {
	It("is stable, node-scoped and DNS-label compatible", func() {
		device := Device{Address: "0000:3b:00.0", VendorID: "10ee", DeviceID: "9038"}

		name := device.GetName("worker-1")
		Expect(name).To(Equal(device.GetName("worker-1")))
		Expect(name).ToNot(Equal(device.GetName("worker-2")))
		Expect(name).To(MatchRegexp(`^pci-[0-9a-f]{40}$`))
	})
})

var _ = Describe("DRA env parsing", func() {
	It("restores claim allocations from container env", func() {
		claimUID := "9f4c8f3a-1111-2222-3333-444455556666"
		envs := []string{
			"PATH=/usr/bin",
			fmt.Sprintf("DRA_PCI_CLAIM_UID_%s=%s", "9F4C8F3A-1111-2222-3333-444455556666", claimUID),
			fmt.Sprintf("DRA_PCI_CLAIM_UID_%s_DEVICE_NAME=pci-abc", "9F4C8F3A-1111-2222-3333-444455556666"),
		}

		allocations, err := parseDraEnvToClaimAllocations(envs)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocations).To(Equal(map[types.UID][]string{
			types.UID(claimUID): {"pci-abc"},
		}))
	})
})
