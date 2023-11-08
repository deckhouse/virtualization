package datasource

import "io"

const (
	// containerDiskImageDir - Expected disk image location in container image as described in
	// https://github.com/kubevirt/kubevirt/blob/main/docs/container-register-disks.md
	containerDiskImageDir = "disk"
)

type DataSourceInterface interface {
	Filename() (string, error)
	Length() (int, error)
	ReadCloser() (io.ReadCloser, error)
	Close() error
}
