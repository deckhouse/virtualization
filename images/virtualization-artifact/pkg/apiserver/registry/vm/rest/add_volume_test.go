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
	"testing"

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/subresources"
)

func hotplugBus(t *testing.T, opts *subresources.VirtualMachineAddVolume) virtv1.AddVolumeOptions {
	t.Helper()
	hook, err := (AddVolumeREST{}).genMutateRequestHook(opts)
	if err != nil {
		t.Fatalf("genMutateRequestHook: %v", err)
	}
	req := &http.Request{}
	if err := hook(req); err != nil {
		t.Fatalf("hook: %v", err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var got virtv1.AddVolumeOptions
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

func TestGenMutateRequestHookBus(t *testing.T) {
	cdrom := hotplugBus(t, &subresources.VirtualMachineAddVolume{VolumeKind: "ClusterVirtualImage", Name: "iso", Image: "img", IsCdrom: true})
	if cdrom.Disk.CDRom == nil {
		t.Fatal("expected CDRom device for cdrom hotplug")
	}
	if cdrom.Disk.CDRom.Bus != virtv1.DiskBusSCSI {
		t.Errorf("cdrom bus = %q, want scsi", cdrom.Disk.CDRom.Bus)
	}

	disk := hotplugBus(t, &subresources.VirtualMachineAddVolume{VolumeKind: "VirtualDisk", Name: "vd", PVCName: "pvc"})
	if disk.Disk.Disk == nil {
		t.Fatal("expected Disk device for non-cdrom hotplug")
	}
	if disk.Disk.Disk.Bus != virtv1.DiskBusSCSI {
		t.Errorf("disk bus = %q, want scsi", disk.Disk.Disk.Bus)
	}
}
