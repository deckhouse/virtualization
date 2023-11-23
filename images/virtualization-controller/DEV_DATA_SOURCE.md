CDI works only with PVC as destination. PVCs are not good for VM images,
we need a separate storage for them. CDI supports import from container image
to PVC, so idea is to use container registry as storage for VM images.

This storage is DVCR: Deckhouse Virtualization Container Registry.

Additional importer and uploader are implemented to import into DVCR instead PVC.

Supported Data Sources:
- HTTP (equals to http source in DataVolume)
- ContainerImage (equals to registry source in DataVolume)
- Upload (equals to upload source in DataVolume)
- VirtualMachineImage (import from DVCR)
- ClusterVirtualMachineImage (import from DVCR)
- VirtualMachineDisk (import from DVCR)
- VirtualMachineDiskSnapshot - not implemented yet
- PersistentVolumeClaim - not implemented yet

Supported storages (destinations):
- Kubernetes - import into PVC.
- ContainerRegistry - import into DVCR.

Possible import paths:
- From Data Source to DVCR: controller will run dvcr-importer or dvcr-uploader.
- From Data Source to PVC: controller will start a 2-phase import:
  - First import into DVCR using dvcr-importer (or dvcr-uploader).
  - Then import DVCR image to the PVC using DataVolume.
- From DVCR to DVCR: controller will run dvcr-importer with custom 'dvcr' source.
- From DVCR to PVC: controller will create DataVolume with the 'registry' source and copy auth Secret and CA bundle ConfigMap.

Import to DVCR 
cvmi_importer.go, vmi_importer.go, vmd_importer.go

Import to PVC
kvbuilder/dv.go

