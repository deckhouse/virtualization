# performance helm chart
## values.yaml
- `resources.vd.spec.type`: 
    - `vi`: creates VMs with VirtualImage in `blockDeviceRefs`
    - `vd`: creates VMs with corresponding `VirtualDisk`
- `resources.vi.spec.type`:
  - `pvc`: create vi with persistentVolumeClaim type