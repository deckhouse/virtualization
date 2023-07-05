package v2alpha1

type BlockDeviceSpec struct {
	Type                BlockDeviceType  `json:"type"`
	VirtualMachineImage *ImageDeviceSpec `json:"virtualMachineImage"`
	VirtualMachineDisk  *DiskDeviceSpec  `json:"virtualMachineDisk"`
}

type BlockDeviceStatus struct {
	Type                BlockDeviceType  `json:"type"`
	VirtualMachineImage *ImageDeviceSpec `json:"virtualMachineImage"`
	VirtualMachineDisk  *DiskDeviceSpec  `json:"virtualMachineDisk"`
	Target              string           `json:"target"`
	Size                string           `json:"size"`
}

type BlockDeviceType string

const (
	ImageDevice BlockDeviceType = "VirtualMachineImage"
	DiskDevice  BlockDeviceType = "VirtualMachineDisk"
)

type ImageDeviceSpec struct {
	Name string `json:"name"`
}

type DiskDeviceSpec struct {
	Name string `json:"name"`
}
