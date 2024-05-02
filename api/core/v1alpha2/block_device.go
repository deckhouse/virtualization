package v1alpha2

type BlockDeviceSpecRef struct {
	Kind BlockDeviceKind `json:"kind"`
	Name string          `json:"name"`
}

type BlockDeviceStatusRef struct {
	Kind         BlockDeviceKind `json:"kind"`
	Name         string          `json:"name"`
	Hotpluggable bool            `json:"hotpluggable"`
	Target       string          `json:"target"`
	Size         string          `json:"size"`
}

type BlockDeviceKind string

const (
	ClusterImageDevice BlockDeviceKind = "ClusterVirtualImage"
	ImageDevice        BlockDeviceKind = "VirtualImage"
	DiskDevice         BlockDeviceKind = "VirtualDisk"
)
