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

This file was initially copied from the Containerized Data Importer (CDI)
project (https://github.com/kubevirt/containerized-data-importer),
Copyright 2018 The CDI Authors, and adapted for the virtualization module.
*/

package image

const (
	// ExtImg is a constant for the .img extenstion
	ExtImg = ".img"
	// ExtIso is a constant for the .iso extenstion
	ExtIso = ".iso"
	// ExtGz is a constant for the .gz extenstion
	ExtGz = ".gz"
	// ExtQcow2 is a constant for the .qcow2 extenstion
	ExtQcow2 = ".qcow2"
	// ExtVmdk is a constant for the .vmdk VMware extenstion
	ExtVmdk = ".vmdk"
	// ExtVdi is a constant for the .vdi VirtualBox extenstion
	ExtVdi = ".vdi"
	// ExtVhd is a constant for the .vhd Microsoft Virtual Server Virtual Hard Disk extenstion
	ExtVhd = ".vhd"
	// ExtVhdx is a constant for the .vhd Hyper-V Virtual Hard Disk V.2 extenstion
	ExtVhdx = ".vhdx"
	// ExtTar is a constant for the .tar extenstion
	ExtTar = ".tar"
	// ExtXz is a constant for the .xz extenstion
	ExtXz = ".xz"
	// ExtZst is a constant for the .zst extenstion
	ExtZst = ".zst"
	// ExtTarXz is a constant for the .tar.xz extenstion
	ExtTarXz = ExtTar + ExtXz
	// ExtTarGz is a constant for the .tar.gz extenstion
	ExtTarGz = ExtTar + ExtGz
)
