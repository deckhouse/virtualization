# performance helm chart
## values.yaml
- `vmDiskType`: 
    - `vi`: creates VMs with VirtualImage in `blockDeviceRefs`
    - `vd`: creates VMs with corresponding `VirtualDisk`
- `viType`:
  - `pvc`: create vi with persistentVolumeClaim type