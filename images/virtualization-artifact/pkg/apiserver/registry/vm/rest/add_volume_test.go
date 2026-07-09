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

package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/subresources"
)

var _ = Describe("AddVolumeREST.genMutateRequestHook", func() {
	mutate := func(opts *subresources.VirtualMachineAddVolume) virtv1.AddVolumeOptions {
		hook, err := AddVolumeREST{}.genMutateRequestHook(opts)
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader("{}"))
		Expect(hook(req)).To(Succeed())

		body, err := io.ReadAll(req.Body)
		Expect(err).NotTo(HaveOccurred())

		var hotplugRequest virtv1.AddVolumeOptions
		Expect(json.Unmarshal(body, &hotplugRequest)).To(Succeed())
		return hotplugRequest
	}

	It("hotplugs a PVC-backed VirtualImage as a read-only disk", func() {
		hotplugRequest := mutate(&subresources.VirtualMachineAddVolume{
			Name:       "vi-image",
			VolumeKind: "VirtualImage",
			PVCName:    "vi-pvc",
		})

		Expect(hotplugRequest.Disk.DiskDevice.Disk).NotTo(BeNil())
		Expect(hotplugRequest.Disk.DiskDevice.Disk.ReadOnly).To(BeTrue())

		pvc := hotplugRequest.VolumeSource.PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		Expect(pvc.ClaimName).To(Equal("vi-pvc"))
		Expect(pvc.ReadOnly).To(BeTrue())
		Expect(pvc.Hotpluggable).To(BeTrue())
	})

	It("hotplugs an ISO PVC-backed VirtualImage as a cdrom with a read-only PVC", func() {
		hotplugRequest := mutate(&subresources.VirtualMachineAddVolume{
			Name:       "vi-image",
			VolumeKind: "VirtualImage",
			PVCName:    "vi-pvc",
			IsCdrom:    true,
		})

		Expect(hotplugRequest.Disk.DiskDevice.CDRom).NotTo(BeNil())
		Expect(hotplugRequest.Disk.DiskDevice.Disk).To(BeNil())

		pvc := hotplugRequest.VolumeSource.PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		Expect(pvc.ReadOnly).To(BeTrue())
	})

	It("hotplugs a registry-backed VirtualImage as a container disk", func() {
		hotplugRequest := mutate(&subresources.VirtualMachineAddVolume{
			Name:       "vi-image",
			VolumeKind: "VirtualImage",
			Image:      "dvcr.example/vi:tag",
		})

		Expect(hotplugRequest.Disk.DiskDevice.Disk).NotTo(BeNil())
		Expect(hotplugRequest.Disk.DiskDevice.Disk.ReadOnly).To(BeFalse())

		cd := hotplugRequest.VolumeSource.ContainerDisk
		Expect(cd).NotTo(BeNil())
		Expect(cd.Image).To(Equal("dvcr.example/vi:tag"))
	})

	It("hotplugs a VirtualDisk as a writable disk", func() {
		hotplugRequest := mutate(&subresources.VirtualMachineAddVolume{
			Name:       "vd-data",
			VolumeKind: "VirtualDisk",
			PVCName:    "vd-pvc",
		})

		Expect(hotplugRequest.Disk.DiskDevice.Disk).NotTo(BeNil())
		Expect(hotplugRequest.Disk.DiskDevice.Disk.ReadOnly).To(BeFalse())

		pvc := hotplugRequest.VolumeSource.PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		Expect(pvc.ClaimName).To(Equal("vd-pvc"))
		Expect(pvc.ReadOnly).To(BeFalse())
	})
})
