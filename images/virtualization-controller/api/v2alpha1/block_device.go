package v2alpha1

type BlockDeviceSpec struct {
	Type                       BlockDeviceType         `json:"type"`
	VirtualMachineImage        *ImageDeviceSpec        `json:"virtualMachineImage"`
	ClusterVirtualMachineImage *ClusterImageDeviceSpec `json:"clusterVirtualMachineImage"`
	VirtualMachineDisk         *DiskDeviceSpec         `json:"virtualMachineDisk"`
}

type BlockDeviceStatus struct {
	Type                       BlockDeviceType         `json:"type"`
	VirtualMachineImage        *ImageDeviceSpec        `json:"virtualMachineImage"`
	ClusterVirtualMachineImage *ClusterImageDeviceSpec `json:"clusterVirtualMachineImage"`
	VirtualMachineDisk         *DiskDeviceSpec         `json:"virtualMachineDisk"`
	Target                     string                  `json:"target"`
	Size                       string                  `json:"size"`
}

type BlockDeviceType string

const (
	ImageDevice        BlockDeviceType = "VirtualMachineImage"
	ClusterImageDevice BlockDeviceType = "ClusterVirtualMachineImage"
	DiskDevice         BlockDeviceType = "VirtualMachineDisk"
)

type ClusterImageDeviceSpec struct {
	Name string `json:"name"`
}

type ImageDeviceSpec struct {
	Name string `json:"name"`
}

type DiskDeviceSpec struct {
	Name string `json:"name"`
}
