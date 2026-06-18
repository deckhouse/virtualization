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

package storageprofile

import (
	corev1 "k8s.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// storageCapability is a supported (accessMode, volumeMode) pair of a provisioner.
type storageCapability struct {
	accessMode corev1.PersistentVolumeAccessMode
	volumeMode corev1.PersistentVolumeMode
}

var (
	rwx   = corev1.ReadWriteMany
	rwo   = corev1.ReadWriteOnce
	rox   = corev1.ReadOnlyMany
	file  = corev1.PersistentVolumeFilesystem
	block = corev1.PersistentVolumeBlock
)

// capabilitiesByProvisioner mirrors CDI's storagecapabilities.CapabilitiesByProvisionerKey.
// CDI has been removed from this module, but the per-provisioner defaults are still required:
// without them every StorageClass would be reported as ReadWriteOnce only, which (for example)
// forces VirtualDisk PVCs backed by replicated.csi.storage.deckhouse.io to ReadWriteOnce even
// though the driver supports ReadWriteMany/Block. That breaks importing a WaitForFirstConsumer
// disk that a VirtualMachine consumes at the same time (the importer pod and the VM pod end up
// on different nodes and cannot share a ReadWriteOnce volume).
//
// The order matters: the first entry is the most preferred capability and is the one selected
// by the disk provisioning logic (see service/volumemode).
var capabilitiesByProvisioner = map[string][]storageCapability{
	// hostpath-provisioner
	"kubevirt.io.hostpath-provisioner": {{rwo, file}},
	"kubevirt.io/hostpath-provisioner": {{rwo, file}},
	"k8s.io/minikube-hostpath":         {{rwo, file}},
	// nfs-csi
	"nfs.csi.k8s.io": {{rwx, file}},
	"k8s-sigs.io/nfs-subdir-external-provisioner": {{rwx, file}},
	// ceph-rbd
	"kubernetes.io/rbd":                  createRbdCapabilities(),
	"rbd.csi.ceph.com":                   createRbdCapabilities(),
	"rook-ceph.rbd.csi.ceph.com":         createRbdCapabilities(),
	"openshift-storage.rbd.csi.ceph.com": createRbdCapabilities(),
	// ceph-fs
	"cephfs.csi.ceph.com":                   {{rwx, file}},
	"openshift-storage.cephfs.csi.ceph.com": {{rwx, file}},
	// LINSTOR
	"linstor.csi.linbit.com": createAllButRWXFileCapabilities(),
	// Deckhouse
	"replicated.csi.storage.deckhouse.io":   createAllButRWXFileCapabilities(),
	"local.csi.storage.deckhouse.io":        createTopoLVMCapabilities(),
	"scsi-generic.csi.storage.deckhouse.io": createAllButRWXFileCapabilities(),
	// DELL Unity XT
	"csi-unity.dellemc.com":     createAllButRWXFileCapabilities(),
	"csi-unity.dellemc.com/nfs": createAllFSCapabilities(),
	// DELL PowerFlex
	"csi-vxflexos.dellemc.com":     createDellPowerFlexCapabilities(),
	"csi-vxflexos.dellemc.com/nfs": createAllFSCapabilities(),
	// DELL PowerScale
	"csi-isilon.dellemc.com": createAllFSCapabilities(),
	// DELL PowerMax
	"csi-powermax.dellemc.com":     createDellPowerMaxCapabilities(),
	"csi-powermax.dellemc.com/nfs": createAllFSCapabilities(),
	// DELL PowerStore
	"csi-powerstore.dellemc.com":     createDellPowerStoreCapabilities(),
	"csi-powerstore.dellemc.com/nfs": createAllFSCapabilities(),
	// storageos
	"kubernetes.io/storageos": {{rwo, file}},
	"storageos":               {{rwo, file}},
	// AWSElasticBlockStore
	"kubernetes.io/aws-ebs": {{rwo, block}},
	"ebs.csi.aws.com":       {{rwo, block}},
	// AWSElasticFileSystem
	"efs.csi.aws.com": {{rwx, file}, {rwo, file}},
	// Azure disk
	"kubernetes.io/azure-disk": {{rwo, block}},
	"disk.csi.azure.com":       {{rwo, block}},
	// Azure file
	"kubernetes.io/azure-file": {{rwx, file}},
	"file.csi.azure.com":       {{rwx, file}},
	// GCE Persistent Disk
	"kubernetes.io/gce-pd":  {{rwo, block}},
	"pd.csi.storage.gke.io": {{rwo, block}},
	// Hitachi
	"hspc.csi.hitachi.com": {{rwx, block}, {rwo, block}, {rwo, file}},
	// HPE
	"csi.hpe.com": {{rwx, block}, {rwo, block}, {rwo, file}},
	// IBM HCI/GPFS2 (Spectrum Scale / Spectrum Fusion)
	"spectrumscale.csi.ibm.com": {{rwx, file}, {rwo, file}},
	// IBM block arrays (FlashSystem)
	"block.csi.ibm.com": {{rwo, block}, {rwo, file}},
	// Portworx in-tree CSI
	"kubernetes.io/portworx-volume/shared": {{rwx, file}},
	"kubernetes.io/portworx-volume":        {{rwo, file}},
	// Portworx CSI
	"pxd.openstorage.org/shared": createOpenStorageSharedVolumeCapabilities(),
	"pxd.openstorage.org":        createOpenStorageSharedVolumeCapabilities(),
	"pxd.portworx.com/shared":    createOpenStorageSharedVolumeCapabilities(),
	"pxd.portworx.com":           createOpenStorageSharedVolumeCapabilities(),
	// Trident
	"csi.trident.netapp.io/ontap-nas": {{rwx, file}, {rwo, file}},
	"csi.trident.netapp.io/ontap-san": {{rwx, block}},
	// topolvm
	"topolvm.cybozu.com": createTopoLVMCapabilities(),
	"topolvm.io":         createTopoLVMCapabilities(),
	// OpenStack Cinder
	"cinder.csi.openstack.org": createRWOBlockAndFilesystemCapabilities(),
	// OpenStack manila
	"manila.csi.openstack.org": {{rwx, file}},
	// ovirt csi
	"csi.ovirt.org": createRWOBlockAndFilesystemCapabilities(),
	// vSphere
	"csi.vsphere.vmware.com":     {{rwo, block}, {rwo, file}},
	"csi.vsphere.vmware.com/nfs": {{rwx, file}, {rwo, block}, {rwo, file}},
	// huawei
	"csi.huawei.com":     createAllButRWXFileCapabilities(),
	"csi.huawei.com/nfs": createAllFSCapabilities(),
	// KubeSAN
	"kubesan.gitlab.io": {{rwx, block}, {rox, block}, {rwo, block}, {rwo, file}},
	// Longhorn
	"driver.longhorn.io":            {{rwo, block}},
	"driver.longhorn.io/migratable": {{rwx, block}, {rwo, block}},
	// Yadro Tatlin
	"csi-tatlinunified.yadro.com": createAllButRWXFileCapabilities(),
}

// claimPropertySetsForProvisioner returns the advised claim property sets for a provisioner,
// preserving preference order. The boolean is false when the provisioner is unknown, in which
// case the caller should fall back to a conservative default.
func claimPropertySetsForProvisioner(provisioner string) ([]cdiv1.ClaimPropertySet, bool) {
	caps, ok := capabilitiesByProvisioner[provisioner]
	if !ok {
		return nil, false
	}

	sets := make([]cdiv1.ClaimPropertySet, 0, len(caps))
	for i := range caps {
		volumeMode := caps[i].volumeMode
		sets = append(sets, cdiv1.ClaimPropertySet{
			AccessModes: []corev1.PersistentVolumeAccessMode{caps[i].accessMode},
			VolumeMode:  &volumeMode,
		})
	}
	return sets, true
}

func createRbdCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, block},
		{rwo, block},
		{rwo, file},
	}
}

func createAllButRWXFileCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
		{rox, file},
	}
}

func createDellPowerMaxCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
	}
}

func createDellPowerFlexCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
		{rox, file},
	}
}

func createDellPowerStoreCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, block},
		{rwo, block},
		{rwo, file},
		{rox, block},
	}
}

func createAllFSCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, file},
		{rwo, file},
		{rox, file},
	}
}

func createTopoLVMCapabilities() []storageCapability {
	return []storageCapability{
		{rwo, block},
		{rwo, file},
	}
}

func createOpenStorageSharedVolumeCapabilities() []storageCapability {
	return []storageCapability{
		{rwx, file},
		{rwo, block},
		{rwo, file},
	}
}

func createRWOBlockAndFilesystemCapabilities() []storageCapability {
	return []storageCapability{
		{rwo, block},
		{rwo, file},
	}
}
